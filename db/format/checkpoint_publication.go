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

// linkCheckpointFile is replaced by tests to exercise the copy fallback.
var linkCheckpointFile = os.Link

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

// checkpointGeneration is a digest plus the files it was computed from.
type checkpointGeneration struct {
	digest            checkpointDigest
	database, sidecar hashedCheckpointFile
}

func hashCheckpointGeneration(path string) (checkpointGeneration, error) {
	database, err := hashCheckpointFile(path)
	if err != nil {
		return checkpointGeneration{}, err
	}
	return hashCheckpointSidecar(path, database)
}

// hashCheckpointSidecar completes a generation whose database is already hashed.
func hashCheckpointSidecar(path string, database hashedCheckpointFile) (checkpointGeneration, error) {
	result := checkpointGeneration{database: database}
	result.digest.Database = database.digest
	sidecar, err := hashCheckpointFile(path + waifIdentitySidecarSuffix)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.sidecar = sidecar
	result.digest.Sidecar = sidecar.digest
	result.digest.HasSidecar = true
	file, err := os.Open(path + waifIdentitySidecarSuffix)
	if err != nil {
		return result, err
	}
	defer file.Close()
	_, err = parseWaifIdentitySidecar(file, result.digest.Database)
	return result, err
}

func checkCheckpointGeneration(path string, expected checkpointDigest) error {
	_, err := verifyCheckpointGeneration(path, expected)
	return err
}

// verifyCheckpointGeneration returns the hashed generation for a later freeze.
func verifyCheckpointGeneration(path string, expected checkpointDigest) (checkpointGeneration, error) {
	for _, member := range []string{path, path + waifIdentitySidecarSuffix} {
		if member != path && !expected.HasSidecar {
			continue
		}
		info, err := os.Lstat(member)
		if err != nil {
			return checkpointGeneration{}, err
		}
		if !info.Mode().IsRegular() {
			return checkpointGeneration{}, fmt.Errorf("checkpoint member is not a regular file: %s", member)
		}
	}
	actual, err := hashCheckpointGeneration(path)
	if err != nil {
		return actual, err
	}
	if actual.digest != expected {
		return actual, fmt.Errorf("checkpoint generation digest mismatch: %s", path)
	}
	return actual, nil
}

// sameCheckpointFile reports whether current is still the file that was hashed.
func sameCheckpointFile(current, hashed os.FileInfo) bool {
	return os.SameFile(current, hashed) && current.Size() == hashed.Size() && current.ModTime().Equal(hashed.ModTime())
}

// freezeCheckpointFile preserves the hashed inode when hard links are supported,
// otherwise copying with bounded memory, verifying the copied bytes against the
// digest as they stream, and syncing the copy before publication. Neither path
// reads the file again: a link is the hashed file, and a copy is hashed in flight.
func freezeCheckpointFile(from, to string, hashed hashedCheckpointFile) (err error) {
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("checkpoint source is not a regular file: %s", from)
	}
	if !sameCheckpointFile(info, hashed.info) {
		return fmt.Errorf("checkpoint source changed after hashing: %s", from)
	}
	if err := linkCheckpointFile(from, to); err == nil {
		linked, err := os.Lstat(to)
		if err != nil {
			return err
		}
		if !sameCheckpointFile(linked, hashed.info) {
			return fmt.Errorf("checkpoint source changed after hashing: %s", from)
		}
		return nil
	}
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer source.Close()
	opened, err := source.Stat()
	if err != nil {
		return err
	}
	if !sameCheckpointFile(opened, hashed.info) {
		return fmt.Errorf("checkpoint source changed after hashing: %s", from)
	}
	target, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, target.Close()) }()
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(target, hash), source); err != nil {
		return err
	}
	var copied [sha256.Size]byte
	if hash.Sum(copied[:0]); copied != hashed.digest {
		return fmt.Errorf("checkpoint copy digest mismatch: %s", from)
	}
	return target.Sync()
}

func freezeCheckpointGeneration(from, to string, generation checkpointGeneration) error {
	if err := freezeCheckpointFile(from, to, generation.database); err != nil {
		return err
	}
	if generation.digest.HasSidecar {
		if err := freezeCheckpointFile(from+waifIdentitySidecarSuffix, to+waifIdentitySidecarSuffix, generation.sidecar); err != nil {
			return err
		}
	}
	return nil
}

func prepareCheckpointJournal(outPath, tempPath string, fs checkpointIO) (checkpointJournal, string, error) {
	staged, err := hashCheckpointGeneration(tempPath)
	if err != nil {
		return checkpointJournal{}, "", err
	}
	return prepareHashedCheckpointJournal(outPath, tempPath, staged, fs)
}

func prepareHashedCheckpointJournal(outPath, tempPath string, staged checkpointGeneration, fs checkpointIO) (journal checkpointJournal, generation string, err error) {
	journal.Version = 1
	journal.Next = staged.digest
	if !journal.Next.HasSidecar {
		return journal, "", fmt.Errorf("staged checkpoint has no WAIF identity sidecar")
	}
	var previous checkpointGeneration
	if _, err := os.Lstat(outPath); err == nil {
		previous, err = hashCheckpointGeneration(outPath)
		if err != nil {
			return journal, "", fmt.Errorf("validate previous checkpoint: %w", err)
		}
		journal.Previous = &previous.digest
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
	if err = freezeCheckpointGeneration(tempPath, filepath.Join(generation, "next"), staged); err != nil {
		return journal, generation, err
	}
	if journal.Previous != nil {
		if err = freezeCheckpointGeneration(outPath, filepath.Join(generation, "previous"), previous); err != nil {
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
	saved, err := verifyCheckpointGeneration(source, digest)
	if err != nil {
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
		if err := freezeCheckpointGeneration(source, restored, saved); err != nil {
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
	staged, err := hashCheckpointGeneration(tempPath)
	if err != nil {
		return fmt.Errorf("prepare checkpoint recovery (staged pair %s): %w", tempPath, err)
	}
	return publishHashedCheckpointPair(outPath, tempPath, staged, fs)
}

// publishHashedCheckpointPair publishes a staged pair whose digest the caller
// computed while writing it.
func publishHashedCheckpointPair(outPath, tempPath string, staged checkpointGeneration, fs checkpointIO) error {
	_, generation, err := prepareHashedCheckpointJournal(outPath, tempPath, staged, fs)
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
