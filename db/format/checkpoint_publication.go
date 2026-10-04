package format

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type checkpointIO struct {
	rename        func(string, string) error
	syncDirectory func(string) error
}

const checkpointJournalSuffix = ".publication"

// Both hashes matter: identical portable dumps can have different WAIF identities.
type checkpointDigest struct {
	Database   [sha256.Size]byte
	Sidecar    [sha256.Size]byte
	HasSidecar bool
}

type checkpointJournal struct {
	Version    int
	Generation string
	Next       checkpointDigest
	Previous   *checkpointDigest `json:",omitempty"`
}

func checkpointGenerationDigest(path string) (checkpointDigest, error) {
	var result checkpointDigest
	var err error
	result.Database, err = hashDatabaseFile(path)
	if err != nil {
		return result, err
	}
	result.Sidecar, err = hashDatabaseFile(path + waifIdentitySidecarSuffix)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.HasSidecar = true
	file, err := os.Open(path + waifIdentitySidecarSuffix)
	if err != nil {
		return result, err
	}
	defer file.Close()
	_, err = parseWaifIdentitySidecar(file, result.Database)
	return result, err
}

func checkCheckpointGeneration(path string, expected checkpointDigest) error {
	for _, member := range []string{path, path + waifIdentitySidecarSuffix} {
		if member != path && !expected.HasSidecar {
			continue
		}
		info, err := os.Lstat(member)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("checkpoint member is not a regular file: %s", member)
		}
	}
	actual, err := checkpointGenerationDigest(path)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("checkpoint generation digest mismatch: %s", path)
	}
	return nil
}

// freezeCheckpointFile preserves an immutable inode when hard links are supported,
// otherwise copying with bounded memory and syncing the copy before publication.
func freezeCheckpointFile(from, to string) (err error) {
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("checkpoint source is not a regular file: %s", from)
	}
	if err := os.Link(from, to); err == nil {
		return nil
	}
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, target.Close()) }()
	if _, err := io.Copy(target, source); err != nil {
		return err
	}
	return target.Sync()
}

func freezeCheckpointGeneration(from, to string, digest checkpointDigest) error {
	if err := freezeCheckpointFile(from, to); err != nil {
		return err
	}
	if digest.HasSidecar {
		if err := freezeCheckpointFile(from+waifIdentitySidecarSuffix, to+waifIdentitySidecarSuffix); err != nil {
			return err
		}
	}
	return checkCheckpointGeneration(to, digest)
}

func prepareCheckpointJournal(outPath, tempPath string, fs checkpointIO) (journal checkpointJournal, generation string, err error) {
	journal.Version = 1
	journal.Next, err = checkpointGenerationDigest(tempPath)
	if err != nil {
		return journal, "", err
	}
	if !journal.Next.HasSidecar {
		return journal, "", fmt.Errorf("staged checkpoint has no WAIF identity sidecar")
	}
	if _, err := os.Lstat(outPath); err == nil {
		previous, err := checkpointGenerationDigest(outPath)
		if err != nil {
			return journal, "", fmt.Errorf("validate previous checkpoint: %w", err)
		}
		journal.Previous = &previous
	} else if !errors.Is(err, os.ErrNotExist) {
		return journal, "", err
	}
	generation, err = os.MkdirTemp(filepath.Dir(outPath), filepath.Base(outPath)+".recovery-")
	if err != nil {
		return journal, "", err
	}
	journal.Generation = filepath.Base(generation)
	// A failed rename may still have changed the filesystem. Once publication
	// is attempted, retain the saved pairs even when that call reports an error.
	journalPublicationAttempted := false
	defer func() {
		if err != nil && !journalPublicationAttempted {
			err = errors.Join(err, os.RemoveAll(generation))
		}
	}()
	if err = freezeCheckpointGeneration(tempPath, filepath.Join(generation, "next"), journal.Next); err != nil {
		return journal, generation, err
	}
	if journal.Previous != nil {
		if err = freezeCheckpointGeneration(outPath, filepath.Join(generation, "previous"), *journal.Previous); err != nil {
			return journal, generation, err
		}
	}
	file, err := os.OpenFile(filepath.Join(generation, "journal"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return journal, generation, err
	}
	encodeErr := json.NewEncoder(file).Encode(journal)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err = errors.Join(encodeErr, syncErr, closeErr); err != nil {
		return journal, generation, err
	}
	if err = fs.syncDirectory(filepath.Join(generation, "journal")); err != nil {
		return journal, generation, err
	}
	// Persist the recovery directory's name before publishing its journal.
	if err = fs.syncDirectory(outPath); err != nil {
		return journal, generation, err
	}
	journalPublicationAttempted = true
	if err = fs.rename(filepath.Join(generation, "journal"), outPath+checkpointJournalSuffix); err != nil {
		return journal, generation, err
	}
	if err = fs.syncDirectory(outPath); err != nil {
		return journal, generation, err
	}
	return journal, generation, nil
}

func readCheckpointJournal(outPath string) (checkpointJournal, string, error) {
	var journal checkpointJournal
	path := outPath + checkpointJournalSuffix
	info, err := os.Lstat(path)
	if err != nil {
		return journal, "", err
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 {
		return journal, "", fmt.Errorf("invalid checkpoint publication journal")
	}
	file, err := os.Open(path)
	if err != nil {
		return journal, "", err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 4097))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&journal); err != nil {
		return journal, "", err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return journal, "", fmt.Errorf("trailing checkpoint publication journal data")
	}
	if journal.Version != 1 || !journal.Next.HasSidecar || journal.Generation != filepath.Base(journal.Generation) ||
		!strings.HasPrefix(journal.Generation, filepath.Base(outPath)+".recovery-") {
		return journal, "", fmt.Errorf("invalid checkpoint publication journal fields")
	}
	generation := filepath.Join(filepath.Dir(outPath), journal.Generation)
	info, err = os.Lstat(generation)
	if err != nil {
		return journal, "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return journal, "", fmt.Errorf("invalid checkpoint recovery directory")
	}
	return journal, generation, nil
}

func finishCheckpointPublication(outPath, generation string, fs checkpointIO) error {
	if err := fs.syncDirectory(outPath); err != nil {
		return err
	}
	if err := os.Remove(outPath + checkpointJournalSuffix); err != nil {
		return err
	}
	if err := fs.syncDirectory(outPath); err != nil {
		return err
	}
	return os.RemoveAll(generation)
}

// recoverCheckpointPair is called while the output's publication lock is held.
func recoverCheckpointPair(outPath string, fs checkpointIO) (resultErr error) {
	journal, generation, err := readCheckpointJournal(outPath)
	if errors.Is(err, os.ErrNotExist) && generation == "" {
		// Absence of the journal is normal; a journal referencing a missing
		// generation is not. Confirm absence at the journal path itself.
		if _, journalErr := os.Lstat(outPath + checkpointJournalSuffix); errors.Is(journalErr, os.ErrNotExist) {
			return nil
		}
	}
	if err != nil {
		return fmt.Errorf("read checkpoint publication journal: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = fmt.Errorf("recover checkpoint (saved pairs %s): %w", generation, resultErr)
		}
	}()
	if err := checkCheckpointGeneration(outPath, journal.Next); err == nil {
		return finishCheckpointPublication(outPath, generation, fs)
	}
	member := "next"
	digest := journal.Next
	if journal.Previous != nil {
		member = "previous"
		digest = *journal.Previous
	}
	source := filepath.Join(generation, member)
	if err := checkCheckpointGeneration(source, digest); err != nil {
		return fmt.Errorf("validate recovery generation %s: %w", source, err)
	}
	if err := checkCheckpointGeneration(outPath, digest); err != nil {
		scratch, err := os.MkdirTemp(generation, "restore-")
		if err != nil {
			return err
		}
		// This scratch directory was created here, never supplied by the journal.
		defer func() { resultErr = errors.Join(resultErr, os.RemoveAll(scratch)) }()
		restored := filepath.Join(scratch, "database")
		if err := freezeCheckpointGeneration(source, restored, digest); err != nil {
			return err
		}
		if err := fs.rename(restored, outPath); err != nil {
			return fmt.Errorf("restore checkpoint database (saved pair %s): %w", source, err)
		}
		if digest.HasSidecar {
			if err := fs.rename(restored+waifIdentitySidecarSuffix, outPath+waifIdentitySidecarSuffix); err != nil {
				return fmt.Errorf("restore checkpoint sidecar (saved pair %s): %w", source, err)
			}
		} else if err := os.Remove(outPath + waifIdentitySidecarSuffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return finishCheckpointPublication(outPath, generation, fs)
}

func publishCheckpointPair(outPath, tempPath string, fs checkpointIO) error {
	_, generation, err := prepareCheckpointJournal(outPath, tempPath, fs)
	if err != nil {
		return fmt.Errorf("prepare checkpoint recovery (staged pair %s, recovery directory %s): %w", tempPath, generation, err)
	}
	publishErr := fs.rename(tempPath, outPath)
	if publishErr == nil {
		publishErr = fs.rename(tempPath+waifIdentitySidecarSuffix, outPath+waifIdentitySidecarSuffix)
	}
	if publishErr == nil {
		publishErr = fs.syncDirectory(outPath)
	}
	if publishErr != nil {
		recoveryErr := recoverCheckpointPair(outPath, fs)
		return fmt.Errorf("publish checkpoint (recovery directory %s): %w", generation, errors.Join(publishErr, recoveryErr))
	}
	if err := finishCheckpointPublication(outPath, generation, fs); err != nil {
		return fmt.Errorf("finish checkpoint publication (recovery directory %s): %w", generation, err)
	}
	return nil
}

// RecoverCheckpoint repairs a journaled publication before the next load/write.
// A valid fully published pair is kept. Otherwise recovery restores the previous
// pair, or completes the first generation when there was no previous checkpoint.
// Without a journal this never alters a database or ignores a sidecar mismatch.
func RecoverCheckpoint(path string) error {
	release, err := lockCheckpointPath(path)
	if err != nil {
		return err
	}
	defer release()
	return recoverCheckpointPair(path, checkpointIO{renameCheckpointFile, syncParentDirectory})
}
