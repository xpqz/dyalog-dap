package transport

// Client manages low-level RIDE transport interactions.
type Client struct{}

// NewClient creates a transport client.
func NewClient() *Client {
	return &Client{}
}
