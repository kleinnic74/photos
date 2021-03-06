package boltstore

import (
	"bitbucket.org/kleinnic74/photos/consts"
	bolt "go.etcd.io/bbolt"
)

// Cursor provides additional convenience functions acround a bolt.Cursor
type Cursor interface {
	Reverse() Cursor
	Skip(count uint) Cursor
	Limit(count uint) Cursor
	First() (key, value []byte)
	Next() (key, value []byte)

	HasMore() bool
}

type baseCursor struct {
	delegate *bolt.Cursor
	limit    int
	skip     int
}

type forwardCursor struct {
	hasMore bool
	baseCursor
}

type reverseCursor struct {
	hasMore bool
	baseCursor
}

func newCursor(delegate *bolt.Cursor, order consts.SortOrder) Cursor {
	switch order {
	case consts.Descending:
		return newReverseCursor(delegate)
	default:
		return newForwardCursor(delegate)
	}
}

func newForwardCursor(delegate *bolt.Cursor) Cursor {
	return &forwardCursor{baseCursor: baseCursor{delegate: delegate, limit: -1}, hasMore: true}
}

func (c *forwardCursor) HasMore() bool {
	return c.hasMore
}

func (c *forwardCursor) Reverse() Cursor {
	return newReverseCursor(c.delegate)
}

func (c *forwardCursor) Skip(count uint) Cursor {
	c.skip = int(count)
	return c
}

func (c *forwardCursor) Limit(count uint) Cursor {
	c.limit = int(count)
	return c
}

func (c *forwardCursor) First() (key []byte, value []byte) {
	var k, v []byte
	for k, v = c.delegate.First(); c.skip > 0 && k != nil; k, v = c.delegate.Next() {
		c.skip--
	}
	c.limit--
	c.hasMore = k != nil
	return k, v
}

func (c *forwardCursor) Next() (key []byte, value []byte) {
	if c.limit == 0 {
		return nil, nil
	}
	k, v := c.delegate.Next()
	for ; c.skip > 0 && k != nil; k, v = c.delegate.Next() {
		c.skip--
	}
	c.limit--
	c.hasMore = k != nil
	return k, v
}

//------------------------------------------------------------------------------

func newReverseCursor(delegate *bolt.Cursor) Cursor {
	return &reverseCursor{baseCursor: baseCursor{delegate: delegate}, hasMore: true}
}

func (c *reverseCursor) Reverse() Cursor {
	return newForwardCursor(c.delegate)
}

func (c *reverseCursor) HasMore() bool {
	return c.hasMore
}

func (c *reverseCursor) First() (key, value []byte) {
	var k, v []byte
	for k, v = c.delegate.Last(); c.skip > 0 && k != nil; k, v = c.delegate.Prev() {
		c.skip--
	}
	c.limit--
	c.hasMore = k != nil
	return k, v
}

func (c *reverseCursor) Limit(count uint) Cursor {
	c.limit = int(count)
	return c
}

func (c *reverseCursor) Skip(count uint) Cursor {
	c.skip = int(count)
	return c
}

func (c *reverseCursor) Next() (key []byte, value []byte) {
	if c.limit == 0 {
		return nil, nil
	}
	k, v := c.delegate.Prev()
	for ; c.skip > 0 && k != nil; k, v = c.delegate.Prev() {
		c.skip--
	}
	c.limit--
	c.hasMore = k != nil
	return k, v
}
