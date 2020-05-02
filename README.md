# uow

![Tests](https://github.com/ruggi/uow/workflows/Tests/badge.svg?branch=master) [![Go Report Card](https://goreportcard.com/badge/github.com/ruggi/uow)](https://goreportcard.com/report/github.com/ruggi/uow)

A proof of concept.

Example usage:

```go
package main

import (
    "github.com/ruggi/uow"
)

func main() {
    db := newDB()       // implements Transactional
    cache := newCache() // implements Transactional

    // Create a new UOW around the desired components.
    unit, err := uow.NewUnitOfWork(db, cache)
    if err != nil {
        panic(err)
    }

    // Run it!
    err := unit.Run(func(c uow.Contextual) error {
        // get from cache
        value, err := cache.Get(c.Context(cache), "key")
        if err != nil {
            return err
        }
        if value != "" {
            // ... do something with the value
            return nil
        }

        // insert in the db
        err = db.Insert(c.Context(db), "key", "value")
        if err != nil {
            return err
        }

        // cache the value
        err = cache.Set(c.Context(cache), "key", "value")
        if err != nil {
            return err
        }

        return nil
    })
}
```

## What it is

This is an experimental approach to Units of Work in Go, without too much hassle or black magic. This is still pretty WIP as I try it out and analyse pros and cons of the choices made.

The idea is to have all-or-nothing units that can either succeed entirely or fail otherwise, rolling back any operation that took place until the failure point.

Let's say there are several components that need to work together:

```go
// Cache is a KV store
type Cache struct{}

func (Cache) Get(ctx context.Context, key string) (string, error) {
    // ...
}

func (Cache) Set(ctx context.Context, key string, value interface{}) error {
    // ...
}

// UserRepository manages model.User
type UserRepository struct{}

func (UserRepository) Find(ctx context.Context, id string) (model.User, error) {
    // ...
}

func (UserRepository) Create(ctx context.Context, email string) (model.User, error) {
    // ...
}

// PostRepository manages model.Post
type PostRepository struct{}

func (PostRepository) Create(ctx context.Context, post model.Post, author model.User) (model.Post, error) {
    // ...
}
```

It's reasonable to assume that, for example, the `Cache` type could be an implementation of a higher-level interface using Redis, while `UserRepository` and `PostRepository` would be implementation of some repository interface attached to a SQL database.

With that being said, let's say how the components above could work together for specific use cases.

The first step is to create such unit of work:

```go
unit, err := uow.NewUnitOfWork(
    cache,
    userRepo,
    postRepo,
)
if err != nil {
    // ...
}
```

All arguments passed to the constructor need to implement the `uow.Transactional` interface, which offers a `Begin` method returning a `Tx` interface:

```go
// Transactional begins a transaction.
type Transactional interface {
    Begin() (Tx, error)
}

// Tx is a set of all-or-nothing operations.
type Tx interface {
    Commit() error
    Rollback() error
}
```

In the case of a `sql.DB` wrapper, the `Tx` insterface is typically implemented by the `sql.Tx` type, and the `Transactional` interface is implemented by the `sql.DB` type itself.

Adjusting one of the example repositories above:

```go
type UserRepository struct {
    db *sql.DB
}

func (u *UserRepository) Begin() (uow.Tx, error) { // makes this a uow.Transactional
    return u.db.Begin()
}
```

At this point, the unit of work can be run, using the `Run` method. The transaction itself, as returned by the `Begin` method of each of the components of the unit of work can be retrieved using the `uow.Contextual` argument of the run function argument.

For example, for a simple "find or create" behavior:

```go
var user model.User

err := unit.Run(func(c uow.Contextual) error {
    userCtx := c.Context(userRepo)

    user, err = userRepo.Find(userCtx, "some-id")
    if err != nil {
        return err
    }
    if user.Valid() {
        return nil
    }

    user, err = userRepo.Create(userCtx, "foo@example.com")
    if err != nil {
        return err
    }

    return nil
})
```

The above will work like a simple `sql.Tx` transaction, as there are no multiple parties involved, but just the `userRepo`. It's easy to extend this to use multiple parties, like the unit of work was declared for.

The actual transaction can be retrieved via the `context.Context` in the components methods, by passing the caller type to the `context.Value` method. For example:

```go
type UserRepository struct{
    db *sql.DB
}

func (u *UserRepository) Find(ctx context.Context, id string) (model.User, error) {
    tx, ok := ctx.Value(u).(*sql.Tx)
    if ok {
        // this is in a unit of work! Use tx for the next operations
    } else {
        // this is not inside a unit of work, use u.db for the next operations
    }
}
```

Let's say that we want a transaction that involves the two repositories, and they both interally use, as expected, the same database, and so we want them to use the same internal transaction when performing a unit of work: to solve this, the repositories can implement the `ContextProvider` interface; if the `ContextProvider` returns the same value for multiple parties, their respective `Begin` function will be only called once for all of them, and the same goes for the `Rollback` and `Commit`. Also, when retrieving the actual transaction via the context, the same transaction will be returned.

To clarify:

```go
var repoContextKey = struct{}{}

func (UserRepository) ContextKey() interface{} { // implements uow.ContextProvider
    return repoContextKey
}

func (PostRepository) ContextKey() interface{} {
    return repoContextKey
}
```

Something good to note:

- Obviously enough, anything can be run inside a unit of work's `Run` method, not just `Transactional` components.
- If one of your components is not transactional per se, you can just leave it out of the `NewUnitOfWork` method.
- If one of your components is an interface implemented by different actual types, you can make the non-transactional implementations be also implementers of the `Transactional` interface, but returning a no-op tx. For convenience, check out the `uow.NopTx` type defined in the package.
