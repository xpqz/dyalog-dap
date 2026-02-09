package sessionstate

// State tracks interpreter session/runtime state.
type State struct{}

// NewState creates session state storage.
func NewState() *State {
	return &State{}
}
