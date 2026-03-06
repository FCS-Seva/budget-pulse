package natsclient

import (
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Client struct {
	Conn      *nats.Conn
	JetStream jetstream.JetStream
}

func New(url string) (*Client, error) {
	conn, err := nats.Connect(url)
	if err != nil {
		return nil, err
	}

	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &Client{
		Conn:      conn,
		JetStream: js,
	}, nil
}

func (c *Client) Close() {
	if c == nil || c.Conn == nil {
		return
	}

	_ = c.Conn.Drain()
	c.Conn.Close()
}
