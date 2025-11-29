package main

import (
	"context"
	"github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
	"log"
	"time"
)

func vault_client(ctx context.Context, path string, secret string) *vault.Response[schema.KvV2ReadResponse] {
	client, err := vault.New(
		vault.WithEnvironment(),
		vault.WithRequestTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}
	result, err := client.Secrets.KvV2Read(
		ctx,
		secret,
		vault.WithMountPath(path),
	)
	if err != nil {
		log.Fatal(err)
	}
	return result
}
