package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	_ "bitbucket.org/kleinnic74/photos/domain"

	"github.com/boltdb/bolt"
)

var (
	dbName = "photos.db"

	libDir string

	bucket string

	keyAcceptor = func(string) bool { return true }

	exe command

	printKey    bool
	printValue  bool
	deleteEntry bool

	commands = []command{
		{"buckets", listBuckets, true},
		{"entries", listEntries, true},
		{"delete", deleteEntries, false},
	}
)

func getCommand(args []string) (command, error) {
	if len(args) == 0 {
		return commands[0], nil
	}
	for i := range commands {
		if args[0] == commands[i].name {
			return commands[i], nil
		}
	}
	return command{}, fmt.Errorf("No such command: %s", args[0])
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s  [options]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.StringVar(&libDir, "l", "gophotos", "Path to photo library")
	flag.StringVar(&bucket, "b", "photos", "Bucket to inspect")
	flag.BoolVar(&printKey, "k", false, "Output keys")
	flag.BoolVar(&printValue, "v", false, "Output value")
	flag.BoolVar(&deleteEntry, "d", false, "Delete entry")
	var keyFilter string
	flag.StringVar(&keyFilter, "kf", "", "Key regex filter")

	flag.Parse()

	var err error
	exe, err = getCommand(flag.Args())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		flag.Usage()
		os.Exit(1)
	}

	if keyFilter != "" {
		keyRE, err := regexp.Compile(keyFilter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Bad key filter RE: %s\n", err)
			os.Exit(2)
		}
		keyAcceptor = keyRE.MatchString
	}
}

type cmdFunc func(*bolt.Tx) error

type command struct {
	name     string
	run      cmdFunc
	readonly bool
}

func listBuckets(tx *bolt.Tx) error {
	return tx.ForEach(func(name []byte, bucket *bolt.Bucket) error {
		fmt.Fprintln(os.Stdout, string(name))
		return nil
	})
}

func listEntries(tx *bolt.Tx) error {
	b := tx.Bucket([]byte(bucket))
	if b == nil {
		return fmt.Errorf("No such bucket: %s", bucket)
	}
	sep, end := "", ""
	var count int
	var badKeys int
	var zeroValues int
	defer func() {
		fmt.Printf("  %d entries\n", count)
		fmt.Printf("  %d bad keys\n", badKeys)
		fmt.Printf("  %d zero values\n", zeroValues)
	}()
	return b.ForEach(func(k, v []byte) error {
		if !keyAcceptor(string(k)) {
			return nil
		}
		count++
		if len(k) == 0 {
			badKeys++
		}
		if len(v) == 0 {
			zeroValues++
		}
		if printKey {
			fmt.Fprintf(os.Stdout, "%s", string(k))
			sep = ":"
			end = "\n"
		}
		if printValue {
			fmt.Fprintf(os.Stdout, "%s%s", sep, string(v))
			end = "\n"
		}
		fmt.Fprintf(os.Stdout, "%s", end)
		return nil
	})
}

func deleteEntries(tx *bolt.Tx) error {
	b := tx.Bucket([]byte(bucket))
	if b == nil {
		return fmt.Errorf("No such bucket: %s", bucket)
	}
	return b.ForEach(func(k, v []byte) error {
		if err := b.Delete(k); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to delete %s\n", string(k))
			return err
		}
		fmt.Fprintf(os.Stderr, "deleted %s\n", string(k))
		return nil
	})
}

func main() {
	var err error
	var db *bolt.DB
	dbPath := filepath.Join(libDir, dbName)
	if db, err = bolt.Open(dbPath, 0600, &bolt.Options{ReadOnly: exe.readonly}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open Bolt DB at %s: %s", dbPath, err)
		os.Exit(1)
	}
	defer db.Close()

	if exe.readonly {
		err = db.View(exe.run)
	} else {
		err = db.Update(exe.run)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while executing: %s", err)
		os.Exit(1)
	}
}
