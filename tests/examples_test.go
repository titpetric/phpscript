package tests

import (
	"context"
	"fmt"
)

func ExampleStorage() {
	ctx := context.WithValue(context.Background(), tenantKey, "acme")
	storage, err := NewStorage(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}

	storage.Set(ctx, "color", "blue")
	record, err := storage.Get(ctx, "color")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(storage.Tenant())
	fmt.Println(record.Key, record.Value)
	fmt.Println(storage.Len())

	// Output:
	// acme
	// color blue
	// 1
}
