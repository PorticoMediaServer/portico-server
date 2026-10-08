//go:build !darwin && !linux

package storage

import (
	"context"
	"io"
	"os"
)

func duplicateRoot(*os.File) (*os.File, error) { return nil, ErrObservedSourceUnsupported }
func (c *Client) RegisterRoot(context.Context, string, string, string, context.Context) (*RootRegistration, error) {
	return nil, ErrObservedSourceUnsupported
}
func registerRootHelper(request, io.Writer) error  { return ErrObservedSourceUnsupported }
func physicalMountIdentity(string) (string, error) { return "", ErrObservedSourceUnsupported }
func sameMountFilesystem(*os.File, string) error   { return ErrObservedSourceUnsupported }
