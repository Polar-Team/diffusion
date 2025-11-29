package main

import (
	"context"
	"github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
	"log"
	"time"
)

type HashicorpVault struct {
	HashicorpVaultIntegration bool   `toml:"enabled"`
	SecretKV2Path             string `toml:"secret_kv2_path"`
	SecretKV2Name             string `toml:"secret_kv2_name"`
	UserNameField             string `toml:"username_field"`
	TokenField                string `toml:"token_field"`
}

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
