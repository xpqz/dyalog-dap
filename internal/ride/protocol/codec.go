package protocol

// Codec handles encoding and decoding RIDE protocol messages.
type Codec struct{}

// NewCodec creates a message codec.
func NewCodec() *Codec {
	return &Codec{}
}
