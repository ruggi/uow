package uow

import (
	"context"
	"fmt"
)

// Transactional begins a transaction.
type Transactional interface {
	Begin() (Tx, error)
}

// Tx is a set of all-or-nothing operations.
type Tx interface {
	Commit() error
	Rollback() error
}

// UnitOfWork wraps a group of Transactional components and can run multiple transactions as one.
type UnitOfWork struct {
	components []Transactional
}

// ContextFunc returns the context for a given Transactional component of a UnitOfWork.
type ContextFunc func(interface{}) context.Context

// Run runs the given function over the UnitOfWork, transactionally.
func (u *UnitOfWork) Run(fn func(ContextFunc) error) (err error) {
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

	contexts := map[interface{}]interface{}{}
	for _, c := range u.components {
		tx, err := c.Begin()
		if err != nil {
			return err
		}
		contexts[c] = tx
		txs = append(txs, tx)
	}

	return fn(func(c interface{}) context.Context {
		return context.WithValue(context.Background(), c, contexts[c])
	})
}

// NewUnitOfWork creates a new UnitOfWork with the given components.
func NewUnitOfWork(components ...Transactional) *UnitOfWork {
	return &UnitOfWork{
		components: components,
	}
}
