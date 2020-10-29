package geocoding

import (
	"context"
	"fmt"

	"bitbucket.org/kleinnic74/photos/domain/gps"
	"bitbucket.org/kleinnic74/photos/library"
	"bitbucket.org/kleinnic74/photos/tasks"

	"github.com/codingsince1985/geo-golang/openstreetmap"
)

var resolver = openstreetmap.Geocoder()

func RegisterTasks(repo *tasks.TaskRepository, geocoder *Geocoder) {
	repo.Register("geoResolve", func() tasks.Task {
		return NewGeoLookupTask(geocoder)
	})
}

type geoLookupTask struct {
	PhotoID  library.PhotoID `json:"photoID"`
	Coords   gps.Coordinates `json:"gps"`
	geocoder *Geocoder
}

func NewGeoLookupTask(geocoder *Geocoder) tasks.Task {
	return geoLookupTask{geocoder: geocoder}
}

func NewGeoLookupTaskWith(g *Geocoder, id library.PhotoID, coords gps.Coordinates) tasks.Task {
	return geoLookupTask{
		PhotoID:  id,
		Coords:   coords,
		geocoder: g,
	}
}

func (t geoLookupTask) Describe() string {
	return fmt.Sprintf("Looking up location of photo %s", t.PhotoID)
}

func (t geoLookupTask) Execute(ctx context.Context, executor tasks.TaskExecutor, lib library.PhotoLibrary) error {
	return t.geocoder.ResolveAndStoreLocation(ctx, t.PhotoID, t.Coords)
}