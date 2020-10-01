package importer

import (
	"context"
	"fmt"
	"os"

	"bitbucket.org/kleinnic74/photos/domain"
	"bitbucket.org/kleinnic74/photos/library"
	"bitbucket.org/kleinnic74/photos/logging"
	"bitbucket.org/kleinnic74/photos/tasks"
	"go.uber.org/zap"
)

type importFileTask struct {
	Path   string `json:"path,omitempty"`
	DryRun bool   `json:"dryrun"`
	Delete bool   `json:"delete,omitempty"`
}

func NewImportFileTask() tasks.Task {
	return &importFileTask{}
}

func NewImportFileTaskWithParams(dryrun bool, path string, deleteAfterImport bool) tasks.Task {
	return &importFileTask{
		Path:   path,
		DryRun: dryrun,
		Delete: deleteAfterImport,
	}
}

func (t importFileTask) Describe() string {
	return fmt.Sprintf("Importing file %s", t.Path)
}

func (t importFileTask) Execute(ctx context.Context, tasks tasks.TaskExecutor, lib library.PhotoLibrary) error {
	log := logging.From(ctx).Named("import")
	img, err := domain.NewPhoto(t.Path)
	if err != nil {
		log.Debug("Skipping", zap.String("file", t.Path), zap.NamedError("cause", err))
		return nil
	}
	log.Info("Found image", zap.String("file", t.Path))
	if t.DryRun {
		return nil
	}
	if err := addToLibrary(ctx, img, lib); err != nil {
		return err
	}

	// TODO: Create thumb

	if t.Delete {
		err = os.Remove(t.Path)
		if err != nil {
			log.Warn("Delete failed", zap.String("file", t.Path), zap.Error(err))
			return err
		}
		log.Info("Deleted file", zap.String("file", t.Path))
	}
	return nil
}

func addToLibrary(ctx context.Context, img domain.Photo, lib library.PhotoLibrary) error {
	content, err := img.Content()
	if err != nil {
		return err
	}
	defer content.Close()
	return lib.Add(ctx, img, content)
}
