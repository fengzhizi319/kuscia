// Copyright 2023 Ant Group Co., Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package http

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"

	"github.com/secretflow/kuscia/pkg/transport/codec"
	"github.com/secretflow/kuscia/pkg/transport/config"
	"github.com/secretflow/kuscia/pkg/transport/msq"
	pb "github.com/secretflow/kuscia/pkg/transport/proto/mesh"
	"github.com/secretflow/kuscia/pkg/transport/transerr"
)

// TestHandleInvokeWithProtobuf tests handling of protobuf format requests
func TestHandleInvokeWithProtobuf(t *testing.T) {
	// Create test server
	httpConfig := config.DefaultServerConfig()
	httpConfig.Port = 2002
	msqConfig := msq.DefaultMsgConfig()

	server := NewServer(httpConfig, msq.NewSessionManager(msqConfig))

	// Create business data
	businessData := []byte("test business data")

	// Create Inbound protobuf message
	inbound := &pb.Inbound{
		Payload: businessData,
		Metadata: map[string]string{
			"test": "value",
		},
	}

	// Serialize to protobuf
	protobufData, err := proto.Marshal(inbound)
	assert.NoError(t, err)

	// Create invoke request with protobuf format
	req, err := http.NewRequest("POST", generatePath(invoke), bytes.NewBuffer(protobufData))
	assert.NoError(t, err)

	req.Header.Set(codec.PtpTopicID, "topic1")
	req.Header.Set(codec.PtpSessionID, "session1")
	req.Header.Set(codec.PtpSourceNodeID, "node0")
	req.Header.Set("Content-Type", "application/x-protobuf")

	// Process request
	outbound := server.handleInvoke(req)
	assert.Equal(t, string(transerr.Success), outbound.Code)

	// Create pop request
	popReq, err := http.NewRequest("POST", generatePath(pop), bytes.NewBuffer(nil))
	assert.NoError(t, err)
	popReq.Header.Set(codec.PtpTopicID, "topic1")
	popReq.Header.Set(codec.PtpSessionID, "session1")
	popReq.Header.Set(codec.PtpTargetNodeID, "node0")

	// Process pop request
	popOutbound := server.handlePop(popReq)
	assert.Equal(t, string(transerr.Success), popOutbound.Code)

	// Verify that returned data is business data, not the entire protobuf message
	assert.Equal(t, businessData, popOutbound.Payload)

	// Ensure it's not nested protobuf data
	nestedInbound := &pb.Inbound{}
	err = proto.Unmarshal(popOutbound.Payload, nestedInbound)
	assert.Error(t, err, "Returned business data should not be a complete Inbound protobuf message")
}

// TestHandleInvokeWithNonProtobuf tests handling of non-protobuf format requests (backward compatibility)
func TestHandleInvokeWithNonProtobuf(t *testing.T) {
	// Create test server
	httpConfig := config.DefaultServerConfig()
	httpConfig.Port = 2003
	msqConfig := msq.DefaultMsgConfig()

	server := NewServer(httpConfig, msq.NewSessionManager(msqConfig))

	// Create raw data
	rawData := []byte("raw test data")

	// Create invoke request with non-protobuf format
	req, err := http.NewRequest("POST", generatePath(invoke), bytes.NewBuffer(rawData))
	assert.NoError(t, err)

	req.Header.Set(codec.PtpTopicID, "topic1")
	req.Header.Set(codec.PtpSessionID, "session1")
	req.Header.Set(codec.PtpSourceNodeID, "node0")
	req.Header.Set("Content-Type", "text/plain") // Non-protobuf format

	// Process request
	outbound := server.handleInvoke(req)
	assert.Equal(t, string(transerr.Success), outbound.Code)

	// Create pop request
	popReq, err := http.NewRequest("POST", generatePath(pop), bytes.NewBuffer(nil))
	assert.NoError(t, err)
	popReq.Header.Set(codec.PtpTopicID, "topic1")
	popReq.Header.Set(codec.PtpSessionID, "session1")
	popReq.Header.Set(codec.PtpTargetNodeID, "node0")

	// Process pop request
	popOutbound := server.handlePop(popReq)
	assert.Equal(t, string(transerr.Success), popOutbound.Code)

	// Verify that returned data is the original raw data
	assert.Equal(t, rawData, popOutbound.Payload)
}
