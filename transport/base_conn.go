package transport

import (
	"io"
	"sync"
	"time"

	"github.com/256dpi/gomqtt/packet"
)

// FlushTimeout is the time after any async writes are flushed asynchronously.
const FlushTimeout = time.Millisecond

// A Carrier is a generalized stream that can be used with BaseConn.
type Carrier interface {
	io.ReadWriteCloser

	SetReadDeadline(time.Time) error
}

// A BaseConn manages the low-level plumbing between the Carrier and the packet
// Stream.
type BaseConn struct {
	carrier Carrier

	stream *packet.Stream

	flushTimer *time.Timer
	flushError error

	sMutex sync.Mutex
	rMutex sync.Mutex

	readTimeout time.Duration
}

// NewBaseConn creates a new BaseConn using the specified Carrier.
func NewBaseConn(c Carrier) *BaseConn {
	return &BaseConn{
		carrier: c,
		stream:  packet.NewStream(c, c),
	}
}

// Send will write the packet to an internal buffer. It will either flush the
// internal buffer immediately or asynchronously in the background when it gets
// stale. Encoding errors are directly returned, but any network errors caught
// while flushing the buffer asynchronously will be returned on the next call.
//
// Note: Only one goroutine can Send at the same time.
func (c *BaseConn) Send(pkt packet.Generic, async bool) error {
	c.sMutex.Lock()
	defer c.sMutex.Unlock()

	// clear and return any error from asyncFlush
	if c.flushError != nil {
		err := c.flushError
		c.flushError = nil
		return err
	}

	// write packet
	err := c.stream.Write(pkt)
	if err != nil {
		// ensure connection gets closed
		c.carrier.Close()

		return err
	}

	// flush immediately if not async
	if !async {
		// stop the timer if existing
		if c.flushTimer != nil {
			c.flushTimer.Stop()
			c.flushTimer = nil
		}

		// perform flush
		err = c.stream.Flush()
		if err != nil {
			// ensure connection gets closed
			c.carrier.Close()

			return err
		}

		return nil
	}

	// setup the timer if missing
	if c.flushTimer == nil {
		c.flushTimer = time.AfterFunc(FlushTimeout, c.asyncFlush)
	}

	return nil
}

func (c *BaseConn) asyncFlush() {
	c.sMutex.Lock()
	defer c.sMutex.Unlock()

	// clear timer
	c.flushTimer = nil

	// flush buffer
	err := c.stream.Flush()
	if err != nil {
		// ensure connection gets closed
		c.carrier.Close()

		// save error if not yet existing
		if c.flushError != nil {
			c.flushError = err
		}
	}
}

// Receive will read from the underlying connection and return a fully read
// packet. It will return an Error if there was an error while decoding or
// reading from the underlying connection.
//
// Note: Only one goroutine can Receive at the same time.
func (c *BaseConn) Receive() (packet.Generic, error) {
	c.rMutex.Lock()
	defer c.rMutex.Unlock()

	// read next packet
	pkt, err := c.stream.Read()
	if err != nil {
		// ensure connection gets closed
		c.carrier.Close()

		return nil, err
	}

	// reset timeout
	c.resetTimeout()

	return pkt, nil
}

// Close will close the underlying connection and cleanup resources. It will
// return an Error if there was an error while closing the underlying
// connection.
func (c *BaseConn) Close() error {
	c.sMutex.Lock()
	defer c.sMutex.Unlock()

	// flush buffer
	err1 := c.stream.Flush()

	// close carrier
	err2 := c.carrier.Close()

	// handle errors
	if err1 != nil {
		return err1
	} else if err2 != nil {
		return err2
	}

	return nil
}

// SetReadLimit sets the maximum size of a packet that can be received.
// If the limit is greater than zero, Receive will close the connection and
// return an Error if receiving the next packet will exceed the limit.
func (c *BaseConn) SetReadLimit(limit int64) {
	c.stream.Decoder.Limit = limit
}

// SetReadTimeout sets the maximum time that can pass between reads.
// If no data is received in the set duration the connection will be closed
// and Read returns an error.
func (c *BaseConn) SetReadTimeout(timeout time.Duration) {
	c.readTimeout = timeout
	c.resetTimeout()
}

func (c *BaseConn) resetTimeout() {
	if c.readTimeout > 0 {
		c.carrier.SetReadDeadline(time.Now().Add(c.readTimeout))
	} else {
		c.carrier.SetReadDeadline(time.Time{})
	}
}
