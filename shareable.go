package jonson

// Shareable defines objects that can be shared between contexts
// and will be passed to new contexts created within existing contexts.
// In case you do have a provided method that needs to be forwarded to new contexts
// created in the current scope, mark them as Shareable:
//
//	type Time struct {
//	  jonson.Shareable
//	  time.Time
//	}
type Shareable interface {
	_isShareable()
}
