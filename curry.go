package main

type curry[T any, P any] struct {
	set bool
	val T
}

func (c *curry[T, P]) Value(parent *P, setter func (*P)T) T {
	if c.set {
		return c.val
	}
	c.set = true
	c.val = setter(parent)
	return c.val
}
