package boltstore

import (
	"context"
	"encoding/json"

	"bitbucket.org/kleinnic74/photos/index"
	"bitbucket.org/kleinnic74/photos/library"
	"bitbucket.org/kleinnic74/photos/logging"
	bolt "go.etcd.io/bbolt"
	"go.uber.org/zap"
)

var (
	indexBucket = []byte("_indextracker")
)

type indexTracker struct {
	db      *bolt.DB
	indexes map[index.Name]library.Version
}

// NewIndexTracker returns a new index tracker using the given BoltDB. The needed
// buckets are created if not yet available.
func NewIndexTracker(db *bolt.DB) (index.Tracker, error) {
	if err := db.Update(func(tx *bolt.Tx) (err error) {
		_, err = tx.CreateBucketIfNotExists(indexBucket)
		return
	}); err != nil {
		return nil, err
	}
	return &indexTracker{db: db, indexes: make(map[index.Name]library.Version)}, nil
}

func (tracker *indexTracker) RegisterIndex(index index.Name, version library.Version) {
	tracker.indexes[index] = version
}

func (tracker *indexTracker) Update(name index.Name, id library.PhotoID, err error) error {
	var status index.Status
	if err != nil {
		status = index.ErrorOnIndex
	} else {
		status = index.Indexed
	}
	version := tracker.indexes[name]
	return tracker.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(indexBucket)
		state := index.NewState()
		if v := b.Get([]byte(id)); v != nil {
			if err := json.Unmarshal(v, &state); err != nil {
				return err
			}
		}
		state.Set(name, status, version)
		stateBytes, err := json.Marshal(state)
		if err != nil {
			return err
		}
		return b.Put([]byte(id), stateBytes)
	})
}

func (tracker *indexTracker) Get(id library.PhotoID) (index.State, bool, error) {
	var found bool
	state := index.NewState()
	err := tracker.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(indexBucket).Get([]byte(id))
		if v == nil {
			return nil
		}
		found = true
		return json.Unmarshal(v, &state)
	})
	return state, found, err
}

func (tracker *indexTracker) GetMissingIndexes(id library.PhotoID) (missing []index.Name, err error) {
	err = tracker.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(indexBucket).Get([]byte(id))
		state := index.NewState()
		if v != nil {
			if err := json.Unmarshal(v, &state); err != nil {
				return err
			}
		}
		for k, version := range tracker.indexes {
			notIndexed := state.StatusFor(k).Status == index.NotIndexed
			outdated := state.StatusFor(k).Version < version
			if notIndexed || outdated {
				missing = append(missing, k)
			}
		}
		return err
	})
	return
}

func (tracker *indexTracker) GetElementStatus(ctx context.Context) (state []index.ElementState, err error) {
	log, ctx := logging.SubFrom(ctx, "indexTracker")
	err = tracker.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(indexBucket)
		return b.ForEach(func(k, v []byte) error {
			indexingStatus := index.NewState()
			if err := json.Unmarshal(v, &indexingStatus); err != nil {
				log.Warn("Failed to JSON decode index statsu", zap.String("photo", string(library.PhotoID(k))), zap.Error(err))
				return nil
			}
			s := index.ElementState{ID: library.PhotoID(k), State: indexingStatus}
			state = append(state, s)
			return nil
		})
	})
	return
}
