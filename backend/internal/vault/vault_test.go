package vault

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVault_CustomKey(t *testing.T) {
	v, _ := NewVault("this-is-a-32-char-custom-key-!!!", "")
	require.NotNil(t, v)
	assert.Len(t, v.masterKey, 32)
}

func TestStoreAndRetrieve(t *testing.T) {
	v, _ := NewVault("test-master-key-32-bytes-long!!!", "")
	ctx := context.Background()

	id, err := v.Store(ctx, "my-secret", "api_key", "connector-1", "supersecretvalue")
	require.NoError(t, err)
	assert.Contains(t, id, "sec-")

	plaintext, err := v.Retrieve(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "supersecretvalue", plaintext)
}

func TestStoreAndRetrieveMultiple(t *testing.T) {
	v, _ := NewVault("test-master-key-32-bytes-long!!!", "")
	ctx := context.Background()

	id1, _ := v.Store(ctx, "secret-1", "api_key", "", "value1")
	id2, _ := v.Store(ctx, "secret-2", "client_secret", "", "value2")

	p1, _ := v.Retrieve(ctx, id1)
	p2, _ := v.Retrieve(ctx, id2)
	assert.Equal(t, "value1", p1)
	assert.Equal(t, "value2", p2)
}

func TestRetrieveNotFound(t *testing.T) {
	v, _ := NewVault("test-master-key-32-bytes-long!!!", "")
	ctx := context.Background()

	_, err := v.Retrieve(ctx, "nonexistent")
	assert.ErrorContains(t, err, "secret not found")
}

func TestDelete(t *testing.T) {
	v, _ := NewVault("test-master-key-32-bytes-long!!!", "")
	ctx := context.Background()

	id, _ := v.Store(ctx, "to-delete", "api_key", "", "value")
	err := v.Delete(ctx, id)
	assert.NoError(t, err)

	_, err = v.Retrieve(ctx, id)
	assert.ErrorContains(t, err, "secret not found")
}

func TestDeleteNotFound(t *testing.T) {
	v, _ := NewVault("test-master-key-32-bytes-long!!!", "")
	ctx := context.Background()

	err := v.Delete(ctx, "nonexistent")
	assert.ErrorContains(t, err, "secret not found")
}

func TestList(t *testing.T) {
	v, _ := NewVault("test-master-key-32-bytes-long!!!", "")
	ctx := context.Background()

	v.Store(ctx, "secret-1", "api_key", "", "value1")
	v.Store(ctx, "secret-2", "client_secret", "", "value2")

	entries := v.List(ctx)
	assert.Len(t, entries, 2)

	for _, e := range entries {
		assert.Equal(t, "[encrypted]", e.Ciphertext, "List should mask ciphertext")
	}
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	v, _ := NewVault("test-master-key-32-bytes-long!!!", "")

	plaintext := []byte("sensitive-data-123")
	ciphertext, err := v.encrypt(plaintext)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, ciphertext)

	decrypted, err := v.decrypt(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestDecryptInvalid(t *testing.T) {
	v, _ := NewVault("test-master-key-32-bytes-long!!!", "")

	_, err := v.decrypt([]byte("too-short"))
	assert.Error(t, err)
}

func TestExportImport(t *testing.T) {
	v, _ := NewVault("test-master-key-32-bytes-long!!!", "")
	ctx := context.Background()

	v.Store(ctx, "secret-1", "api_key", "", "value1")

	data, err := v.Export()
	require.NoError(t, err)
	assert.Contains(t, string(data), "secret-1")

	v2, _ := NewVault("test-master-key-32-bytes-long!!!", "")
	err = v2.Import(data)
	require.NoError(t, err)
	assert.Len(t, v2.secrets, 1)

	entries := v2.List(ctx)
	require.Len(t, entries, 1)
	p, err := v2.Retrieve(ctx, entries[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "value1", p)
}

func TestSave_NoPath(t *testing.T) {
	v, _ := NewVault("test-master-key-32-bytes-long!!!", "")
	err := v.Save()
	assert.ErrorContains(t, err, "no vault path configured")
}

func TestSaveAndLoadFromFile(t *testing.T) {
	tmpFile, _ := os.CreateTemp(t.TempDir(), "vault-*.json")
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	v, _ := NewVault("test-master-key-32-bytes-long!!!", tmpFile.Name())
	ctx := context.Background()
	v.Store(ctx, "persisted-secret", "api_key", "", "persisted-value")

	err := v.Save()
	require.NoError(t, err)

	data, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	assert.Contains(t, string(data), "persisted-secret")
}

func TestConcurrency(t *testing.T) {
	v, _ := NewVault("test-master-key-32-bytes-long!!!", "")
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			v.Store(ctx, "concurrent", "api_key", "", "value")
			v.List(ctx)
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 100; i++ {
			v.Store(ctx, "concurrent", "api_key", "", "value")
			v.List(ctx)
		}
		done <- struct{}{}
	}()

	for i := 0; i < 2; i++ {
		<-done
	}
	// No race — if it didn't panic, the mutex works
	assert.GreaterOrEqual(t, len(v.List(ctx)), 1)
}
