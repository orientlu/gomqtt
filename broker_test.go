// Copyright (c) 2014 The gomqtt Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package broker

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gomqtt/client"
	"github.com/gomqtt/packet"
	"github.com/gomqtt/spec"
	"github.com/gomqtt/tools"
	"github.com/gomqtt/transport"
	"github.com/stretchr/testify/assert"
)

func TestBrokerTCP(t *testing.T) {
	testBroker(t, "tcp")
}

func TestBrokerWS(t *testing.T) {
	testBroker(t, "ws")
}

func testBroker(t *testing.T, protocol string) {
	backend := NewMemoryBackend()
	backend.Logins = map[string]string{
		"allow": "allow",
	}

	port, done := Run(NewWithBackend(backend), protocol)

	config := spec.AllFeatures()
	config.URL = fmt.Sprintf("%s://allow:allow@localhost:%s", protocol, port.Port())
	config.DenyURL = fmt.Sprintf("%s://deny:deny@localhost:%s", protocol, port.Port())

	spec.Run(t, config)

	close(done)
}

func TestConnectTimeout(t *testing.T) {
	broker := New()
	broker.ConnectTimeout = 10 * time.Millisecond

	port, done := Run(broker, "tcp")

	conn, err := transport.Dial(port.URL())
	assert.NoError(t, err)

	pkt, err := conn.Receive()
	assert.Nil(t, pkt)
	assert.Error(t, err)

	close(done)
}

func TestKeepAlive(t *testing.T) {
	t.Parallel()

	port, done := Run(New(), "tcp")

	opts := client.NewOptions()
	opts.KeepAlive = "1s"

	client := client.New()

	var reqCounter int32
	var respCounter int32

	client.Logger = func(message string) {
		if strings.Contains(message, "Pingreq") {
			atomic.AddInt32(&reqCounter, 1)
		} else if strings.Contains(message, "Pingresp") {
			atomic.AddInt32(&respCounter, 1)
		}
	}

	connectFuture, err := client.Connect(port.URL(), opts)
	assert.NoError(t, err)
	assert.NoError(t, connectFuture.Wait())
	assert.Equal(t, packet.ConnectionAccepted, connectFuture.ReturnCode)
	assert.False(t, connectFuture.SessionPresent)

	time.Sleep(2500 * time.Millisecond)

	err = client.Disconnect()
	assert.NoError(t, err)

	assert.Equal(t, int32(2), atomic.LoadInt32(&reqCounter))
	assert.Equal(t, int32(2), atomic.LoadInt32(&respCounter))

	close(done)
}

func TestKeepAliveTimeout(t *testing.T) {
	t.Parallel()

	connect := packet.NewConnectPacket()
	connect.KeepAlive = 1

	connack := packet.NewConnackPacket()

	client := tools.NewFlow().
		Send(connect).
		Receive(connack).
		End()

	port, done := Run(New(), "tcp")

	conn, err := transport.Dial(port.URL())
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	client.Test(t, conn)

	close(done)
}
