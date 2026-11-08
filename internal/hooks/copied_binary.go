package hooks

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/storage"
)

func hostCopiedBinaryPath(hooksDir string) string {
	unixPath, windowsPath := copiedBinaryPaths(hooksDir)
	if runtime.GOOS == "windows" {
		return windowsPath
	}
	return unixPath
}

func copiedBinaryProofPath(binaryPath string) string {
	return binaryPath + ".sha256"
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func readCopiedBinaryProof(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("copied executable ownership record is not a regular file: %s", path)
	}
	if info.Size() != 65 {
		return "", false, fmt.Errorf("invalid copied executable ownership record: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	if len(data) != 65 || data[64] != '\n' {
		return "", false, fmt.Errorf("invalid copied executable ownership record: %s", path)
	}
	if _, err := hex.DecodeString(string(data[:64])); err != nil || strings.ToLower(string(data[:64])) != string(data[:64]) {
		return "", false, fmt.Errorf("invalid copied executable ownership record: %s", path)
	}
	return string(data[:64]), true, nil
}

func copiedBinaryOwnership(binaryPath string) (exists bool, owned bool, err error) {
	info, err := os.Lstat(binaryPath)
	if err != nil && !os.IsNotExist(err) {
		return false, false, err
	}
	exists = err == nil
	if exists && !info.Mode().IsRegular() {
		return true, false, fmt.Errorf("copied executable is not a regular file: %s", binaryPath)
	}
	proof, recorded, err := readCopiedBinaryProof(copiedBinaryProofPath(binaryPath))
	if err != nil {
		return exists, false, err
	}
	if !exists {
		if recorded {
			return false, false, fmt.Errorf("copied executable ownership record exists without its executable: %s", binaryPath)
		}
		return false, false, nil
	}
	if !recorded {
		return true, false, nil
	}
	actual, err := fileDigest(binaryPath)
	if err != nil {
		return true, false, err
	}
	return true, proof == actual, nil
}

func validateCopiedBinaryDestination(binaryPath string, force bool) error {
	exists, owned, err := copiedBinaryOwnership(binaryPath)
	if err != nil {
		return err
	}
	if exists && !owned && !force {
		return fmt.Errorf("copied executable has no matching ownership record: %s (inspect it and use --force to overwrite)", binaryPath)
	}
	return nil
}

func persistCopiedBinaryProof(binaryPath, digest string) error {
	path := copiedBinaryProofPath(binaryPath)
	data := []byte(digest + "\n")
	if _, err := os.Lstat(path); err == nil {
		return storage.WriteFile(path, data, 0o600)
	} else if os.IsNotExist(err) {
		return storage.CreateFile(path, data, 0o600)
	} else {
		return err
	}
}

func restoreCopiedBinaryProof(binaryPath, previous string, existed bool) error {
	if !existed {
		return removeOwnedFile(copiedBinaryProofPath(binaryPath))
	}
	return persistCopiedBinaryProof(binaryPath, previous)
}
