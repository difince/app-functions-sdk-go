//
// Copyright (c) 2019 Intel Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package edgexsdk

import (
	"github.com/edgexfoundry/app-functions-sdk-go/internal/telemetry"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/edgexfoundry/app-functions-sdk-go/internal/webserver"

	"os"
	"os/signal"
	"syscall"

	"github.com/edgexfoundry/app-functions-sdk-go/internal"
	"github.com/edgexfoundry/app-functions-sdk-go/internal/common"
	"github.com/edgexfoundry/app-functions-sdk-go/internal/common/runtime"
	"github.com/edgexfoundry/app-functions-sdk-go/internal/trigger"
	"github.com/edgexfoundry/app-functions-sdk-go/internal/trigger/http"
	"github.com/edgexfoundry/app-functions-sdk-go/internal/trigger/messagebus"
	"github.com/edgexfoundry/app-functions-sdk-go/pkg/excontext"
	"github.com/edgexfoundry/app-functions-sdk-go/pkg/transforms"
	logger "github.com/edgexfoundry/go-mod-core-contracts/clients/logging"
	"github.com/edgexfoundry/go-mod-core-contracts/models"
	registry "github.com/edgexfoundry/go-mod-registry"
	"github.com/edgexfoundry/go-mod-registry/pkg/factory"
)

// AppFunctionsSDK ...
type AppFunctionsSDK struct {
	transforms     []func(edgexcontext excontext.Context, params ...interface{}) (bool, interface{})
	ServiceKey     string
	configProfile  string
	configDir      string
	useRegistry    bool
	httpErrors     chan error
	webserver      *webserver.WebServer
	registryClient registry.Client
	config         common.ConfigurationStruct
	LoggingClient  logger.LoggingClient
}

// SetPipeline defines the order in which each function will be called as each event comes in.
func (sdk *AppFunctionsSDK) SetPipeline(transforms ...func(edgexcontext excontext.Context, params ...interface{}) (bool, interface{})) error {
	if len(transforms) == 0 {
		return errors.New("No transforms provided to pipeline")
	}
	sdk.transforms = transforms
	return nil
}

// FilterByDeviceID ...
func (sdk *AppFunctionsSDK) FilterByDeviceID(deviceIDs []string) func(excontext.Context, ...interface{}) (bool, interface{}) {
	transforms := transforms.Filter{
		FilterValues: deviceIDs,
	}
	return transforms.FilterByDeviceID
}

// FilterByValueDescriptor ...
func (sdk *AppFunctionsSDK) FilterByValueDescriptor(valueIDs []string) func(excontext.Context, ...interface{}) (bool, interface{}) {
	transforms := transforms.Filter{
		FilterValues: valueIDs,
	}
	return transforms.FilterByValueDescriptor
}

// TransformToXML ...
func (sdk *AppFunctionsSDK) TransformToXML() func(excontext.Context, ...interface{}) (bool, interface{}) {
	transforms := transforms.Conversion{}
	return transforms.TransformToXML
}

// TransformToJSON ...
func (sdk *AppFunctionsSDK) TransformToJSON() func(excontext.Context, ...interface{}) (bool, interface{}) {
	transforms := transforms.Conversion{}
	return transforms.TransformToJSON
}

// HTTPPost will add an export function that sends data from the previous function to the specified Endpoint via http POST. Passing an empty string to the mimetype
// method will default to application/json ...
func (sdk *AppFunctionsSDK) HTTPPost(url string, mimeType string) func(excontext.Context, ...interface{}) (bool, interface{}) {
	transforms := transforms.HTTPSender{
		URL:      url,
		MimeType: mimeType,
	}
	return transforms.HTTPPost
}

// HTTPPostJSON will add an export function that sends data from the previous function to the specified Endpoint via http POST with a mime type of application/json.
func (sdk *AppFunctionsSDK) HTTPPostJSON(url string) func(excontext.Context, ...interface{}) (bool, interface{}) {
	return sdk.HTTPPost(url, "application/json")
}

// HTTPPostXML will add an export function that sends data from the previous function to the specified Endpoint via http POST with a mime type of application/xml.
func (sdk *AppFunctionsSDK) HTTPPostXML(url string) func(excontext.Context, ...interface{}) (bool, interface{}) {
	return sdk.HTTPPost(url, "application/xml")
}

// MQTTSend ...
func (sdk *AppFunctionsSDK) MQTTSend(addr models.Addressable, cert string, key string) func(excontext.Context, ...interface{}) (bool, interface{}) {
	// transforms := transforms.MQTTSender{
	// 	URL: url,
	// }
	sender := transforms.NewMQTTSender(sdk.LoggingClient, addr, cert, key)

	return sender.MQTTSend
}

//MakeItRun the SDK
func (sdk *AppFunctionsSDK) MakeItRun() {
	// a little telemetry where?
	httpErrors := make(chan error)
	defer close(httpErrors)

	//determine which runtime to load
	runtime := runtime.GolangRuntime{Transforms: sdk.transforms}

	sdk.webserver = &webserver.WebServer{
		Config:        &sdk.config,
		LoggingClient: sdk.LoggingClient,
	}
	sdk.webserver.ConfigureStandardRoutes()

	// determine input type and create trigger for it
	trigger := sdk.setupTrigger(sdk.config, runtime)

	// Initialize the trigger (i.e. start a web server, or connect to message bus)
	trigger.Initialize(sdk.LoggingClient)

	sdk.webserver.StartHTTPServer(sdk.httpErrors)
	c := <-sdk.httpErrors

	sdk.LoggingClient.Warn(fmt.Sprintf("Terminating: %v", c))
	os.Exit(0)

}

func (sdk *AppFunctionsSDK) ApplicationSettings() map[string] string {
	return sdk.config.ApplicationSettings
}

func (sdk *AppFunctionsSDK) setupTrigger(configuration common.ConfigurationStruct, runtime runtime.GolangRuntime) trigger.ITrigger {
	var trigger trigger.ITrigger
	// Need to make dynamic, search for the binding that is input

	switch strings.ToUpper(configuration.Binding.Type) {
	case "HTTP":
		sdk.LoggingClient.Info("Loading Http Trigger")
		trigger = &http.Trigger{Configuration: configuration, Runtime: runtime, Webserver: sdk.webserver}
	case "MESSAGEBUS":
		sdk.LoggingClient.Info("Loading messageBus Trigger")
		trigger = &messagebus.Trigger{Configuration: configuration, Runtime: runtime}
	}

	return trigger
}

// Initialize will parse command line flags, register for interrupts, initalize the logging system, and ingest configuration.
func (sdk *AppFunctionsSDK) Initialize() error {

	flag.BoolVar(&sdk.useRegistry, "registry", false, "Indicates the service should use the registry.")
	flag.BoolVar(&sdk.useRegistry, "r", false, "Indicates the service should use registry.")

	flag.StringVar(&sdk.configProfile, "profile", "", "Specify a profile other than default.")
	flag.StringVar(&sdk.configProfile, "p", "", "Specify a profile other than default.")

	flag.StringVar(&sdk.configDir, "confdir", "", "Specify an alternate configuration directory.")
	flag.StringVar(&sdk.configDir, "c", "", "Specify an alternate configuration directory.")

	flag.Parse()

	now := time.Now()
	until := now.Add(time.Millisecond * time.Duration(internal.BootTimeoutDefault))
	for now.Before(until) {
		err := sdk.initializeConfiguration()
		if err != nil {
			fmt.Printf("failed to initialize Registry: %v\n", err)
		} else {
			//initialize logger
			sdk.LoggingClient = logger.NewClient("AppFunctionsSDK", false, "./test.txt", sdk.config.Writable.LogLevel)
			sdk.LoggingClient.Info("Configuration and logger successfully initialized")
			break
		}

		time.Sleep(time.Second * time.Duration(1))
	}

	if sdk.useRegistry {
		go sdk.listenForConfigChanges()
	}

	// Handles SIGINT/SIGTERM and exits gracefully
	sdk.listenForInterrupts()
	
	go telemetry.StartCpuUsageAverage()

	return nil
}

func (sdk *AppFunctionsSDK) initializeConfiguration() error {

	// Currently have to load configuration from filesystem first in order to obtain Registry Host/Port
	configuration := &common.ConfigurationStruct{}
	err := common.LoadFromFile(sdk.configProfile, sdk.configDir, configuration)
	if err != nil {
		return err
	}
	sdk.config = *configuration

	if sdk.useRegistry {
		registryConfig := registry.Config{
			Host:          sdk.config.Registry.Host,
			Port:          sdk.config.Registry.Port,
			Type:          sdk.config.Registry.Type,
			Stem:          internal.ConfigRegistryStem,
			CheckInterval: "1s",
			CheckRoute:    internal.ApiPingRoute,
			ServiceKey:    sdk.ServiceKey,
			ServiceHost:   sdk.config.Service.Host,
			ServicePort:   sdk.config.Service.Port,
		}

		client, err := factory.NewRegistryClient(registryConfig)
		if err != nil {
			return fmt.Errorf("connection to Registry could not be made: %v", err)
		}
		//set registryClient
		sdk.registryClient = client

		if !sdk.registryClient.IsAlive() {
			return fmt.Errorf("registry (%s) is not running", registryConfig.Type)
		}

		// Register the service with Registry
		err = sdk.registryClient.Register()
		if err != nil {
			return fmt.Errorf("could not register service with Registry: %v", err)
		}

		hasConfig, err := sdk.registryClient.HasConfiguration()
		if err != nil {
			return fmt.Errorf("could not determine if registry has configuration: %v", err)
		}

		if hasConfig {
			rawConfig, err := sdk.registryClient.GetConfiguration(configuration)
			if err != nil {
				return fmt.Errorf("could not get configuration from Registry: %v", err)
			}

			actual, ok := rawConfig.(*common.ConfigurationStruct)
			if !ok {
				return fmt.Errorf("configuration from Registry failed type check")
			}

			sdk.config = *actual
			//Check that information was successfully read from Consul
			if sdk.config.Service.Port == 0 {
				sdk.LoggingClient.Error("Error reading from registry")
			}

			fmt.Println("Configuration loaded from registry")
		} else {
			err := sdk.registryClient.PutConfiguration(sdk.config, true)
			if err != nil {
				return fmt.Errorf("could not push configuration into registry: %v", err)
			}
			fmt.Println("Configuration pushed to registry")
		}

	}

	return nil
}

func (sdk *AppFunctionsSDK) listenForConfigChanges() {

	updateChannel := make(chan interface{})

	defer close(updateChannel)

	sdk.LoggingClient.Info("Listening for changes from registry")
	sdk.registryClient.WatchForChanges(updateChannel, sdk.httpErrors, &common.WritableInfo{}, internal.WritableKey)

	for {
		select {
		case err := <-sdk.httpErrors:
			sdk.LoggingClient.Error(err.Error())

		case raw, ok := <-updateChannel:
			if !ok {
				sdk.LoggingClient.Error("Failed to receive changes from update channel")
				return
			}

			actual, ok := raw.(*common.WritableInfo)
			if !ok {
				sdk.LoggingClient.Error("listenForConfigChanges() type check failed")
				return
			}

			sdk.config.Writable = *actual

			sdk.LoggingClient.Info("Writeable configuration has been updated from Registry")
			sdk.LoggingClient.SetLogLevel(sdk.config.Writable.LogLevel)

			// TODO: Deal with pub/sub topics may have changed. Save copy of writeable so that we can determine what if anything changed?
		}
	}
}

func (sdk *AppFunctionsSDK) listenForInterrupts() {
	sdk.LoggingClient.Info("Listening for interrupts")
	go func() {
		signals := make(chan os.Signal)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

		signalReceived := <-signals
		sdk.LoggingClient.Info("Terminating: " + signalReceived.String())
		sdk.httpErrors <- fmt.Errorf("%s", <-signals)
		os.Exit(0)
	}()
}
