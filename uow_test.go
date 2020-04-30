package uow_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/ruggi/uow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tx struct {
	value       string
	committed   bool
	rolledBack  bool
	commitErr   error
	rollbackErr error
}

func (t *tx) Commit() error {
	t.committed = true
	return t.commitErr
}

func (t *tx) Rollback() error {
	t.rolledBack = true
	return t.rollbackErr
}

type foo struct {
	tx       tx
	value    string
	beginErr error
	panic    interface{}
	err      error
}

func (f *foo) Begin() (uow.Tx, error) {
	return &f.tx, f.beginErr
}

func (f *foo) Foo(ctx context.Context) (string, error) {
	if f.panic != nil {
		panic(f.panic)
	}
	val := f.value
	if tx, ok := ctx.Value(f).(*tx); ok {
		val = tx.value
	}
	return val, f.err
}

func TestNewUOW(t *testing.T) {
	type nonTransactional struct{}
	_, err := uow.NewUnitOfWork(nonTransactional{})
	require.Error(t, err)
	assert.Equal(t, "cannot create unit of work: component uow_test.nonTransactional does not implement uow.Transactional", err.Error())

	_, err = uow.NewUnitOfWork(&foo{}, foo{})
	require.Error(t, err)
	assert.Equal(t, "cannot create unit of work: component uow_test.foo does not implement uow.Transactional", err.Error())

	_, err = uow.NewUnitOfWork(&foo{}, &foo{})
	require.NoError(t, err)
}

func TestUOWRun(t *testing.T) {
	tests := []struct {
		a             *foo
		b             *foo
		shouldError   bool
		expectedError error
		txaCommitted  bool
		txbCommitted  bool
		txaRolledBack bool
		txbRolledBack bool
		expectedValue string
	}{
		{
			a:             &foo{value: "a", tx: tx{value: "tx a"}},
			b:             &foo{value: "b", tx: tx{value: "tx b"}},
			shouldError:   false,
			expectedError: nil,
			txaCommitted:  true,
			txaRolledBack: false,
			txbCommitted:  true,
			txbRolledBack: false,
			expectedValue: "tx b",
		},
		{
			a:             &foo{value: "a", tx: tx{value: "tx a"}, beginErr: fmt.Errorf("begin err")},
			b:             &foo{value: "b", tx: tx{value: "tx b"}},
			shouldError:   true,
			expectedError: fmt.Errorf("begin err"),
			txaCommitted:  false,
			txaRolledBack: false,
			txbCommitted:  false,
			txbRolledBack: false,
			expectedValue: "",
		},
		{
			a:             &foo{value: "a", tx: tx{value: "tx a"}},
			b:             &foo{value: "b", tx: tx{value: "tx b"}, beginErr: fmt.Errorf("begin err")},
			shouldError:   true,
			expectedError: fmt.Errorf("begin err"),
			txaCommitted:  false,
			txaRolledBack: true,
			txbCommitted:  false,
			txbRolledBack: false,
			expectedValue: "",
		},
		{
			a:             &foo{value: "a", tx: tx{value: "tx a"}, err: fmt.Errorf("boom a")},
			b:             &foo{value: "b", tx: tx{value: "tx b"}},
			shouldError:   true,
			expectedError: fmt.Errorf("boom a"),
			txaCommitted:  false,
			txaRolledBack: true,
			txbCommitted:  false,
			txbRolledBack: true,
			expectedValue: "tx a",
		},
		{
			a:             &foo{value: "a", tx: tx{value: "tx a"}, err: fmt.Errorf("boom a")},
			b:             &foo{value: "b", tx: tx{value: "tx b"}, err: fmt.Errorf("boom b")},
			shouldError:   true,
			expectedError: fmt.Errorf("boom a"),
			txaCommitted:  false,
			txaRolledBack: true,
			txbCommitted:  false,
			txbRolledBack: true,
			expectedValue: "tx a",
		},
		{
			a:             &foo{value: "a", tx: tx{value: "tx a"}},
			b:             &foo{value: "b", tx: tx{value: "tx b"}, err: fmt.Errorf("boom b")},
			shouldError:   true,
			expectedError: fmt.Errorf("boom b"),
			txaCommitted:  false,
			txaRolledBack: true,
			txbCommitted:  false,
			txbRolledBack: true,
			expectedValue: "tx b",
		},
		{
			a:             &foo{value: "a", tx: tx{value: "tx a", commitErr: fmt.Errorf("a commit err")}},
			b:             &foo{value: "b", tx: tx{value: "tx b"}},
			shouldError:   true,
			expectedError: fmt.Errorf("a commit err"),
			txaCommitted:  true,
			txaRolledBack: true,
			txbCommitted:  false,
			txbRolledBack: true,
			expectedValue: "tx b",
		},
		{
			a:             &foo{value: "a", tx: tx{value: "tx a"}},
			b:             &foo{value: "b", tx: tx{value: "tx b", commitErr: fmt.Errorf("b commit err")}},
			shouldError:   true,
			expectedError: fmt.Errorf("b commit err"),
			txaCommitted:  true,
			txaRolledBack: true,
			txbCommitted:  true,
			txbRolledBack: true,
			expectedValue: "tx b",
		},
		{
			a:             &foo{value: "a", tx: tx{value: "tx a"}},
			b:             &foo{value: "b", tx: tx{value: "tx b"}, panic: "boom"},
			shouldError:   true,
			expectedError: fmt.Errorf("recovered: boom"),
			txaCommitted:  false,
			txaRolledBack: true,
			txbCommitted:  false,
			txbRolledBack: true,
			expectedValue: "tx a",
		},
		{
			a:             &foo{value: "a", tx: tx{value: "tx a"}},
			b:             &foo{value: "b", tx: tx{value: "tx b"}, panic: fmt.Errorf("boom")},
			shouldError:   true,
			expectedError: fmt.Errorf("boom"),
			txaCommitted:  false,
			txaRolledBack: true,
			txbCommitted:  false,
			txbRolledBack: true,
			expectedValue: "tx a",
		},
	}

	for _, tt := range tests {
		result := ""

		unit, err := uow.NewUnitOfWork(tt.a, tt.b)
		require.NoError(t, err)

		err = unit.Run(func(ctx uow.ContextFunc) error {
			var err error
			result, err = tt.a.Foo(ctx(tt.a))
			if err != nil {
				return err
			}
			result, err = tt.b.Foo(ctx(tt.b))
			if err != nil {
				return err
			}
			return nil
		})
		if tt.shouldError {
			require.Error(t, err)
			assert.Equal(t, tt.expectedError.Error(), err.Error())
		} else {
			assert.NoError(t, err)
		}

		assert.Equal(t, tt.txaCommitted, tt.a.tx.committed)
		assert.Equal(t, tt.txbCommitted, tt.b.tx.committed)
		assert.Equal(t, tt.txaRolledBack, tt.a.tx.rolledBack)
		assert.Equal(t, tt.txbRolledBack, tt.b.tx.rolledBack)
		assert.Equal(t, tt.expectedValue, result)
	}
}
