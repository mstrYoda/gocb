package gocb

// Scope represents a single scope within a bucket.
type Scope struct {
	sb stateBlock
}

func newScope(bucket *Bucket, scopeName string) *Scope {
	scope := &Scope{
		sb: bucket.stateBlock(),
	}
	scope.sb.ScopeName = scopeName
	return scope
}

func (s *Scope) clone() *Scope {
	newS := *s
	return &newS
}

// Name returns the name of the scope.
func (s *Scope) Name() string {
	return s.sb.ScopeName
}

// Collection returns an instance of a collection.
// Volatile: This API is subject to change at any time.
func (s *Scope) Collection(collectionName string, opts *CollectionOptions) *Collection {
	return newCollection(s, collectionName, opts)
}

func (s *Scope) stateBlock() stateBlock {
	return s.sb
}
