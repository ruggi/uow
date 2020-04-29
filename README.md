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
    db := newDB()       // Implements Transactional
    cache := newCache() // implements Transactional

    err := uow.NewUnitOfWork(db, cache).Run(func(ctx uow.ContextFunc) error {
        // get from cache
        value, err := cache.Get(ctx(cache), "key")
        if err != nil {
            return err
        }
        if value != "" {
            // ... do something with the value
            return nil
        }

        // insert in the db
        err = db.Insert(ctx(db), "key", "value")
        if err != nil {
            return err
        }

        // cache the value
        err = cache.Set(ctx(cache), "key", "value")
        if err != nil {
            return err
        }

        return nil
    })
}
```
