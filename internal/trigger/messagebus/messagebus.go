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

package messagebus

import (
	"github.com/edgexfoundry/app-functions-sdk-go/internal/common"
	"github.com/edgexfoundry/app-functions-sdk-go/internal/common/runtime"
	logger "github.com/edgexfoundry/go-mod-core-contracts/clients/logging"
)

// Trigger implements ITrigger to support MessageBusData
type Trigger struct {
	Configuration common.ConfigurationStruct
	Runtime       runtime.GolangRuntime
	outputData    interface{}
}

// Initialize ...
func (mb *Trigger) Initialize(logger logger.LoggingClient) error {
	return nil
}

// Complete ...
func (mb *Trigger) Complete(outputData string) {
	//
	mb.outputData = outputData
}
