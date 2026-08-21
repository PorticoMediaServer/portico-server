//go:build windows

package app

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsDPAPIEnvelopeLimit = 64 << 10

type dpapiSecretKeyProvider struct {
	path string
	mu   sync.Mutex
}

// NewSecretKeyProvider protects the random wrapping key with Windows DPAPI's
// current-user scope. Only the DPAPI blob is persisted outside the database;
// the usable wrapping key never enters settings or a portable database backup.
func NewSecretKeyProvider(appDataDir string) SecretKeyProvider {
	return &dpapiSecretKeyProvider{path: filepath.Join(appDataDir, "keys", "hosted-authority.key.dpapi")}
}

func (p *dpapiSecretKeyProvider) ProviderKind() SecretProviderKind {
	return SecretProviderKindDPAPI
}

func (p *dpapiSecretKeyProvider) Seal(plaintext []byte) (SecretEnvelope, error) {
	if p == nil {
		return SecretEnvelope{}, newSecretProviderError(SecretProviderKindDPAPI, "seal", SecretProviderStateUnavailable, ErrSecretKeyUnavailable)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	key, err := p.loadOrCreateKey()
	if err != nil {
		return SecretEnvelope{}, secretProviderErrorFor(SecretProviderKindDPAPI, "seal", err)
	}
	return sealSecretEnvelope(SecretProviderKindDPAPI, key, plaintext)
}

func (p *dpapiSecretKeyProvider) Open(envelope SecretEnvelope) ([]byte, error) {
	if p == nil {
		return nil, newSecretProviderError(SecretProviderKindDPAPI, "open", SecretProviderStateUnavailable, ErrSecretKeyUnavailable)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := validateSecretEnvelope(envelope); err != nil {
		return nil, newSecretProviderError(SecretProviderKindDPAPI, "open", SecretProviderStateEnvelopeInvalid, err)
	}
	key, err := p.loadProtectedKey()
	if err != nil {
		return nil, secretProviderErrorFor(SecretProviderKindDPAPI, "open", err)
	}
	return openSecretEnvelope(SecretProviderKindDPAPI, key, envelope)
}

func (p *dpapiSecretKeyProvider) loadOrCreateKey() ([]byte, error) {
	key, err := p.loadProtectedKey()
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, ErrSecretKeyMissing) {
		return nil, err
	}
	if strings.TrimSpace(p.path) == "" {
		return nil, ErrSecretKeyUnavailable
	}
	if err := ensureDPAPIKeyDirectory(filepath.Dir(p.path)); err != nil {
		return nil, err
	}
	key = make([]byte, secretEnvelopeKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	protected, err := dpapiProtect(key)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(p.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return p.loadProtectedKey()
		}
		return nil, err
	}
	removeOnFailure := true
	defer func() {
		if removeOnFailure {
			_ = os.Remove(p.path)
		}
	}()
	if _, err := file.Write(protected); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	removeOnFailure = false
	return p.loadProtectedKey()
}

func (p *dpapiSecretKeyProvider) loadProtectedKey() ([]byte, error) {
	if p == nil || strings.TrimSpace(p.path) == "" {
		return nil, ErrSecretKeyUnavailable
	}
	info, err := os.Lstat(p.path)
	if os.IsNotExist(err) {
		return nil, ErrSecretKeyMissing
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, ErrSecretKeyCorrupt
	}
	file, err := os.Open(p.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		return nil, fmt.Errorf("%w: DPAPI blob changed while opening", ErrSecretKeyCorrupt)
	}
	protected, err := io.ReadAll(io.LimitReader(file, windowsDPAPIEnvelopeLimit+1))
	if err != nil {
		return nil, err
	}
	if len(protected) == 0 || len(protected) > windowsDPAPIEnvelopeLimit {
		return nil, ErrSecretKeyCorrupt
	}
	key, err := dpapiUnprotect(protected)
	if err != nil {
		return nil, err
	}
	if len(key) != secretEnvelopeKeySize {
		return nil, ErrSecretKeyCorrupt
	}
	return key, nil
}

func ensureDPAPIKeyDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: DPAPI key directory is not a real directory", ErrSecretKeyCorrupt)
	}
	return nil
}

func dpapiProtect(key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, fmt.Errorf("%w: empty key", ErrSecretKeyCorrupt)
	}
	input := windows.DataBlob{Size: uint32(len(key)), Data: &key[0]}
	var output windows.DataBlob
	if err := windows.CryptProtectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output); err != nil {
		return nil, fmt.Errorf("%w: DPAPI protect failed: %v", ErrSecretKeyUnavailable, err)
	}
	if output.Data == nil || output.Size == 0 || output.Size > windowsDPAPIEnvelopeLimit {
		return nil, fmt.Errorf("%w: DPAPI returned an invalid protected key", ErrSecretKeyCorrupt)
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))))
	protected := make([]byte, int(output.Size))
	copy(protected, unsafe.Slice(output.Data, int(output.Size)))
	return protected, nil
}

func dpapiUnprotect(protected []byte) ([]byte, error) {
	if len(protected) == 0 {
		return nil, ErrSecretKeyCorrupt
	}
	input := windows.DataBlob{Size: uint32(len(protected)), Data: &protected[0]}
	var output windows.DataBlob
	var description *uint16
	if err := windows.CryptUnprotectData(&input, &description, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output); err != nil {
		return nil, fmt.Errorf("%w: DPAPI key is unavailable to this user or machine", ErrSecretKeyUnavailable)
	}
	if description != nil {
		defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(description))))
	}
	if output.Data == nil || output.Size == 0 || output.Size > windowsDPAPIEnvelopeLimit {
		return nil, fmt.Errorf("%w: DPAPI returned an invalid unprotected key", ErrSecretKeyCorrupt)
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))))
	key := make([]byte, int(output.Size))
	copy(key, unsafe.Slice(output.Data, int(output.Size)))
	return key, nil
}
