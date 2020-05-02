package uow

import (
	"context"
	"fmt"
)

// Transactional begins a transaction.
type Transactional interface {
	Begin() (Tx, error)
}

// Tx commits or rolls back a set of all-or-nothing operations.
type Tx interface {
	Commit() error
	Rollback() error
}

// UnitOfWork wraps a group of Transactional components and can run multiple transactions as one.
type UnitOfWork struct {
	components []Transactional
	contexts   map[interface{}]interface{}
}

// NewUnitOfWork creates a new UnitOfWork with the given components. The passed components must implement the Transactional interface.
func NewUnitOfWork(components ...interface{}) (*UnitOfWork, error) {
	unit := &UnitOfWork{
		components: make([]Transactional, 0, len(components)),
		contexts:   map[interface{}]interface{}{},
	}
	for _, c := range components {
		t, ok := c.(Transactional)
		if !ok {
			return nil, fmt.Errorf("cannot create unit of work: component %T does not implement uow.Transactional", c)
		}
		unit.components = append(unit.components, t)
	}
	return unit, nil
}

// Context returns the context for the given argument.
func (u *UnitOfWork) Context(c interface{}) context.Context {
	return context.WithValue(context.Background(), c, u.contexts[c])
}

// Contextual returns a context for a given argument.
type Contextual interface {
	Context(interface{}) context.Context
}

// ContextProvider returns a context key
type ContextProvider interface {
	ContextKey() interface{}
}

// Run runs the given function over the UnitOfWork, transactionally.
func (u *UnitOfWork) Run(fn func(Contextual) error) (err error) {
	txs := make([]Tx, 0, len(u.components))

	defer func() {
		if err == nil {
			return
		}
		for _, tx := range txs {
			rbErr := tx.Rollback()
			if rbErr != nil {
				// ... do something about it
			}
		}
	}()

	defer func() {
		if err != nil {
			return
		}
		for _, tx := range txs {
			err = tx.Commit()
			if err != nil { // good job, you broke it!
				return
			}
		}
	}()

	defer func() {
		rec := recover()
		if rec != nil {
			switch t := rec.(type) {
			case error:
				err = t
			default:
				err = fmt.Errorf("recovered: %v", t)
			}
		}
	}()

	for _, c := range u.components {
		var key interface{} = c
		if cp, ok := c.(ContextProvider); ok {
			key = cp.ContextKey()
		}
		if _, ok := u.contexts[key]; ok { // make sure that the same context providers share the same context
			continue
		}
		tx, err := c.Begin()
		if err != nil {
			return err
		}
		u.contexts[key] = tx
		txs = append(txs, tx)
	}

	return fn(u)
}

// NopTx is a no-op transaction that can be used to implement temporary/dummy Transactional types.
type NopTx struct{}

// Commit does nothing.
func (NopTx) Commit() error { return nil }

// Rollback does nothing.
func (NopTx) Rollback() error { return nil }
