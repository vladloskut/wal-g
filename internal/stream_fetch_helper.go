package internal

import (
	"fmt"
	"io"
	"time"

	"github.com/klauspost/readahead"
	"github.com/wal-g/wal-g/pkg/storages/storage"

	"github.com/wal-g/tracelog"
	"github.com/wal-g/wal-g/internal/compression"
	"github.com/wal-g/wal-g/utility"
)

func ParseTS(endTSEnvVar string) (endTS *time.Time, err error) {
	endTSStr, ok := GetSetting(endTSEnvVar)
	if ok {
		t, err := time.Parse(time.RFC3339, endTSStr)
		if err != nil {
			return nil, err
		}
		endTS = &t
	}
	return endTS, nil
}

// TODO : unit tests
// GetLogsDstSettings reads from the environment variables fetch settings
func GetLogsDstSettings(operationLogsDstEnvVariable string) (dstFolder string, err error) {
	dstFolder, ok := GetSetting(operationLogsDstEnvVariable)
	if !ok {
		return dstFolder, NewUnsetRequiredSettingError(operationLogsDstEnvVariable)
	}
	return dstFolder, nil
}

// TODO : unit tests
// downloadAndDecompressStream downloads, decompresses and writes stream to stdout
func DownloadAndDecompressStream(backup Backup, writeCloser io.WriteCloser) error {
	defer utility.LoggedClose(writeCloser, "")

	for _, decompressor := range compression.Decompressors {
		archiveReader, exists, err := TryDownloadFile(backup.Folder, GetStreamName(backup.Name, decompressor.FileExtension()))
		if err != nil {
			return fmt.Errorf("failed to dowload file: %w", err)
		}
		if !exists {
			continue
		}

		tracelog.DebugLogger.Printf("Found file: %s.%s", backup.Name, decompressor.FileExtension())
		err = DecompressDecryptBytes(&EmptyWriteIgnorer{WriteCloser: writeCloser}, archiveReader, decompressor)
		if err != nil {
			return fmt.Errorf("failed to decompress and decrypt file: %w", err)
		}
		return nil
	}
	return newArchiveNonExistenceError(fmt.Sprintf("Archive '%s' does not exist.\n", backup.Name))
}

// TODO : unit tests
// DownloadAndDecompressStream downloads, decompresses and writes stream to stdout
func DownloadAndDecompressSplittedStream(backup Backup, partitions int, blockSize int, extension string, writeCloser io.WriteCloser) error {
	defer utility.LoggedClose(writeCloser, "")

	decompressor := compression.GetDecompressor(extension)
	if decompressor == nil {
		return fmt.Errorf("decompressor for file type '%s' not found", extension)
	}

	errorsPerWorker := make([]chan error, 0)
	writers, done := storage.MergeWriter(EmptyWriteIgnorer{WriteCloser: writeCloser}, partitions, blockSize)

	for i := 0; i < partitions; i++ {
		fileName := GetPartitionedStreamName(backup.Name, decompressor.FileExtension(), i)
		errCh := make(chan error)
		errorsPerWorker = append(errorsPerWorker, errCh)

		go func(fileName string, errCh chan error, writer io.WriteCloser) {
			defer close(errCh)

			archiveReader, exists, err := TryDownloadFile(backup.Folder, fileName)
			if err != nil {
				tracelog.ErrorLogger.PrintOnError(writer.Close())
				errCh <- fmt.Errorf("failed to dowload file: %w", err)
				return
			}
			if !exists {
				errCh <- writer.Close()
				return
			}
			tracelog.DebugLogger.Printf("Found files: %s", fileName)

			decryptReadCloser, err := DecryptBytes(archiveReader)
			if err != nil {
				errCh <- fmt.Errorf("failed to decrypt file: %w", err)
				return
			}

			// readahead will start separate goroutine that puts CPU-heavy operations (Decrypt and Decompress) into
			// different goroutines
			asyncDecryptReadCloser := readahead.NewReadCloser(decryptReadCloser)

			err = decompressor.Decompress(writer, asyncDecryptReadCloser)
			if err != nil {
				errCh <- fmt.Errorf("failed to decompress archive reader: %w", err)
				return
			}
			errCh <- writer.Close()
		}(fileName, errCh, writers[i])
	}

	var lastErr error
	for _, ch := range errorsPerWorker {
		select {
		case err := <-ch:
			tracelog.ErrorLogger.PrintOnError(err)
			if err != nil {
				lastErr = err
			}
		case err := <-done:
			tracelog.ErrorLogger.PrintOnError(err)
			return err
		}
	}

	// wait until MergeWriter flushes its caches
	err := <-done
	if err != nil && lastErr == nil {
		lastErr = err
	}

	return lastErr
}
