// photos project main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/boltdb/bolt"
	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"bitbucket.org/kleinnic74/photos/consts"
	"bitbucket.org/kleinnic74/photos/geocoding"
	"bitbucket.org/kleinnic74/photos/importer"
	"bitbucket.org/kleinnic74/photos/library"
	"bitbucket.org/kleinnic74/photos/library/boltstore"
	"bitbucket.org/kleinnic74/photos/logging"
	"bitbucket.org/kleinnic74/photos/rest"
	"bitbucket.org/kleinnic74/photos/rest/wdav"
	"bitbucket.org/kleinnic74/photos/tasks"
)

var (
	dbName = "photos.db"

	libDir string
	uiDir  string
	port   uint

	logger *zap.Logger
	ctx    context.Context
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s  [options]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.StringVar(&libDir, "l", "gophotos", "Path to photo library")
	flag.StringVar(&uiDir, "ui", "", "Path to the frontend static assets")
	flag.UintVar(&port, "p", 8080, "HTTP server port")
	ctx = logging.Context(context.Background(), nil)
	logger = logging.From(ctx)

	flag.Parse()

	absdir, err := filepath.Abs(libDir)
	if err != nil {
		logger.Fatal("Could not determine path", zap.String("dir", libDir), zap.Error(err))
	}
	libDir = absdir
}

func main() {
	//	classifier := NewEventClassifier()
	if err := os.MkdirAll(libDir, os.ModePerm); err != nil {
		log.Fatal("Failed to create directory", zap.String("dir", libDir), zap.Error(err))
	}

	taskRepo := tasks.NewTaskRepository()
	tasks.RegisterTasks(taskRepo)
	importer.RegisterTasks(taskRepo)

	db, err := bolt.Open(filepath.Join(libDir, dbName), 0600, nil)
	if err != nil {
		logger.Fatal("Failed to initialize library", zap.Error(err))
	}
	defer db.Close()
	indexTracker, err := boltstore.NewIndexTracker(db)
	if err != nil {
		logger.Fatal("Failed to initialize library", zap.Error(err))
	}
	store, err := boltstore.NewBoltStore(db)
	if err != nil {
		logger.Fatal("Failed to initialize library", zap.Error(err))
	}
	lib, err := library.NewBasicPhotoLibrary(libDir, store)
	if err != nil {
		logger.Fatal("Failed to initialize library", zap.Error(err))
	}
	logger.Info("Opened photo library", zap.String("path", libDir))
	geoindex, err := boltstore.NewBoltGeoIndex(db)
	if err != nil {
		logger.Fatal("Failed to initialize geoindex", zap.Error(err))
	}
	geocoding.RegisterTasks(taskRepo, geoindex)
	RegisterDBUpgradeTasks(taskRepo, lib)

	dateindex, err := boltstore.NewDateIndex(db)
	if err != nil {
		logger.Fatal("Failed to initialize dataindex", zap.Error(err))
	}

	executor := tasks.NewSerialTaskExecutor(lib)
	executorContext, cancelExecutor := context.WithCancel(ctx)
	go executor.DrainTasks(executorContext)

	indexer := NewIndexer(indexTracker, executor)
	indexer.RegisterDirect("date", boltstore.DateIndexVersion, dateindex.Add)
	indexer.RegisterDefered("geo", boltstore.GeoIndexVersion, geocoding.LookupPhotoOnAdd(geoindex))

	indexer.RegisterTasks(taskRepo)

	lib.AddCallback(indexer.Add)

	go launchStartupTasks(ctx, taskRepo, executor)

	// REST Handlers
	router := mux.NewRouter()

	photoApp := rest.NewApp(lib)
	photoApp.InitRoutes(router)

	timeline := rest.NewTimelineHandler(dateindex, lib)
	timeline.InitRoutes(router)

	geo := rest.NewGeoHandler(geoindex, lib)
	geo.InitRoutes(router)

	tasksApp := rest.NewTaskHandler(taskRepo, executor)
	tasksApp.InitRoutes(router)

	tmpdir := filepath.Join(libDir, "tmp")
	wdav, err := wdav.NewWebDavHandler(tmpdir, backgroundImport(executor))
	if err != nil {
		logger.Fatal("Error initializing webdav interface", zap.Error(err))
	}
	router.PathPrefix("/dav/").Handler(wdav)
	if consts.IsDevMode() && uiDir != "" {
		router.PathPrefix("/").Handler(http.FileServer(http.Dir(uiDir)))
	} else {
		router.PathPrefix("/").Handler(rest.Embedder())
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	signalContext, cancel := context.WithCancel(ctx)

	go func() {
		oscall := <-c
		logger.Info("Received signal", zap.Any("signal", oscall))
		cancel()
	}()

	if ifs, err := net.Interfaces(); err == nil {
		for _, intf := range ifs {
			if addr, err := intf.Addrs(); err == nil {
				for _, a := range addr {
					ip, _, _ := net.ParseCIDR(a.String())
					if ip.IsLoopback() || !ip.IsGlobalUnicast() {
						continue
					}
					logger.Info("Address", zap.String("if", intf.Name),
						zap.String("net", a.Network()),
						zap.String("addr", a.String()),
						zap.Bool("loopback", ip.IsLoopback()),
						zap.Bool("global", ip.IsGlobalUnicast()))
				}
			}
		}
	}
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: rest.WithMiddleWares(router, "rest"),
	}
	go func() {
		logger.Info("Starting HTTP server...", zap.Uint("port", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP server failed", zap.Error(err))
		}
		logger.Info("HTTP server stopped")
	}()

	<-signalContext.Done()

	logger.Info("Stopping server...")

	ctxShutdown, cancelServerShutdown := context.WithTimeout(ctx, 5*time.Second)
	defer func() {
		cancelServerShutdown()
	}()
	if err := server.Shutdown(ctxShutdown); err != nil {
		logger.Fatal(("Failed to shutdown HTTP server"), zap.Error(err))
	}

	cancelExecutor()

	logger.Info("Terminated gracefully")

	// img := classifier.DistanceMatrixToImage()
	// log.Printf("Creating time-distance matrix image %s", matrixFilename)
	// out, err := os.Create(matrixFilename)
	// if err != nil {
	// 	log.Fatalf("Could not create distance matrix: %s", err)
	// }
	// defer out.Close()
	// png.Encode(out, img)
}

func launchStartupTasks(ctx context.Context, tasksRepo *tasks.TaskRepository, executor tasks.TaskExecutor) {
	for _, t := range tasksRepo.DefinedTasks() {
		if t.RunOnStart {
			logging.From(ctx).Debug("Launching startup task", zap.String("task", t.Name))
			task, err := tasksRepo.CreateTask(t.Name)
			if err != nil {
				logging.From(ctx).Warn("StartupTasks", zap.Error(err))
				continue
			}
			executor.Submit(ctx, task)
		}
	}
}

func backgroundImport(executor tasks.TaskExecutor) wdav.UploadedFunc {
	return func(ctx context.Context, path string) {
		task := importer.NewImportFileTaskWithParams(false, path, true)
		if _, err := executor.Submit(ctx, task); err != nil {
			logging.From(ctx).Warn("Could not import file", zap.String("path", path), zap.Error(err))
		}
	}
}
