package format

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/MongooseMoo/barn/types"
)

const waifIdentitySidecarSuffix = ".waifids"

// databaseHashObserver, when set by tests, sees each whole-file hash pass.
var databaseHashObserver func(path string)

// hashedCheckpointFile binds a digest to the file that produced it, so a
// hard link of that same file can be trusted without hashing it again.
type hashedCheckpointFile struct {
	digest [sha256.Size]byte
	info   os.FileInfo
}

// hashCheckpointFile hashes all file bytes with bounded temporary storage.
func hashCheckpointFile(path string) (hashedCheckpointFile, error) {
	var result hashedCheckpointFile
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer file.Close()
	if databaseHashObserver != nil {
		databaseHashObserver(path)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return result, err
	}
	hash.Sum(result.digest[:0])
	result.info, err = file.Stat()
	return result, err
}

// hashDatabaseFile hashes all database bytes with bounded temporary storage.
func hashDatabaseFile(path string) ([sha256.Size]byte, error) {
	hashed, err := hashCheckpointFile(path)
	return hashed.digest, err
}

func readWaifIdentitySidecar(databasePath string) ([]types.WaifIdentity, error) {
	path := databasePath + waifIdentitySidecarSuffix
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open WAIF identity sidecar: %w", err)
	}
	defer file.Close()

	digest, err := hashDatabaseFile(databasePath)
	if err != nil {
		return nil, fmt.Errorf("hash database for WAIF identity sidecar: %w", err)
	}
	return parseWaifIdentitySidecar(file, digest)
}

// A caller that has already hashed a checkpoint can validate its sidecar
// without another full database scan.
func parseWaifIdentitySidecar(file io.Reader, digest [sha256.Size]byte) ([]types.WaifIdentity, error) {
	wantHeader := fmt.Sprintf("barn-waif-identities-v1 %x", digest)
	var identities []types.WaifIdentity
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() || scanner.Text() != wantHeader {
		return nil, fmt.Errorf("WAIF identity sidecar does not match database")
	}
	for scanner.Scan() {
		encoded := strings.TrimSpace(scanner.Text())
		if encoded == "" {
			return nil, fmt.Errorf("empty WAIF identity sidecar entry %d", len(identities))
		}
		identity, err := types.ParseWaifIdentity(encoded)
		if err != nil {
			return nil, fmt.Errorf("parse WAIF identity sidecar entry %d: %w", len(identities), err)
		}
		identities = append(identities, identity)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read WAIF identity sidecar: %w", err)
	}
	return identities, nil
}

func writeWaifIdentitySidecar(path, databasePath string, identities []types.WaifIdentity) error {
	_, err := writeHashedWaifIdentitySidecar(path, databasePath, identities)
	return err
}

// writeHashedWaifIdentitySidecar also returns the database hash so checkpoint
// publication can journal the staged generation without another full scan.
func writeHashedWaifIdentitySidecar(path, databasePath string, identities []types.WaifIdentity) (hashedCheckpointFile, error) {
	database, err := hashCheckpointFile(databasePath)
	if err != nil {
		return database, fmt.Errorf("hash database for WAIF identity sidecar: %w", err)
	}
	return database, writeWaifIdentitySidecarFile(path, database.digest, identities)
}

func writeWaifIdentitySidecarFile(path string, digest [sha256.Size]byte, identities []types.WaifIdentity) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create WAIF identity sidecar: %w", err)
	}
	if _, err := fmt.Fprintf(file, "barn-waif-identities-v1 %x\n", digest); err != nil {
		file.Close()
		return fmt.Errorf("write WAIF identity sidecar header: %w", err)
	}
	for _, identity := range identities {
		if _, err := fmt.Fprintln(file, identity); err != nil {
			file.Close()
			return fmt.Errorf("write WAIF identity sidecar: %w", err)
		}
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync WAIF identity sidecar: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close WAIF identity sidecar: %w", err)
	}
	return nil
}
