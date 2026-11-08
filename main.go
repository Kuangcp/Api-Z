package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	mygoui "github.com/egoist/mygo/ui"

	"api-z/app"
	"api-z/ui"
)

func main() {
	var (
		showVersion = flag.Bool("version", false, "打印版本后退出")
		dataDir     = flag.String("data-dir", "", "api-z 数据目录（默认 XDG data）")
		configDir   = flag.String("config-dir", "", "api-z 配置目录（默认 XDG config）")
		apiXHome    = flag.String("api-x-home", "", "api-x 数据目录（默认自动识别）")
		doMigrate   = flag.Bool("migrate", false, "只读识别并迁移 api-x 数据后退出")
	)
	flag.Parse()
	if *showVersion {
		fmt.Println("api-z 0.1.0")
		return
	}

	paths, err := app.DefaultPaths()
	if err != nil {
		log.Fatal(err)
	}
	if *dataDir != "" || *configDir != "" {
		if *dataDir == "" {
			*dataDir = paths.DataDir
		}
		if *configDir == "" {
			*configDir = paths.ConfigDir
		}
		paths = app.PathsAt(*dataDir, *configDir)
	}
	if err := paths.Ensure(); err != nil {
		log.Fatal(err)
	}
	if err := app.InitLogs(paths.LogDir); err != nil {
		log.Printf("日志初始化失败: %v", err)
	}
	defer app.CloseLogs()

	store, err := app.OpenStore(paths.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	src := *apiXHome
	if src == "" {
		src = app.ApiXDataDir()
	}
	var stats app.MigrateStats
	if store.Meta("migrated_from") == "" {
		if _, err := os.Stat(filepath.Join(src, "collections.db")); err == nil {
			stats, err = app.MigrateFromApiX(store, paths, src)
			if err != nil && !errors.Is(err, app.ErrDestinationNotEmpty) {
				app.LogWarn("migrate", "failed", "source", src, "err", err.Error())
			}
		} else {
			app.LogInfo("startup", "no api-x data found", "source", src)
		}
	}
	if *doMigrate {
		fmt.Printf("source=%s collections=%d folders=%d requests=%d artifacts=%d exchanges=%d environments=%d\n",
			stats.Source, stats.Collections, stats.Folders, stats.Requests,
			stats.Artifacts, stats.Exchanges, stats.Environments)
		return
	}

	tree, err := store.LoadTree()
	if err != nil {
		log.Fatal(err)
	}
	env, err := store.LoadEnvironments()
	if err != nil {
		app.LogWarn("startup", "environments load failed", "err", err.Error())
	}
	app.LogInfo("startup", "api-z start", "data", paths.DataDir, "db", paths.DBPath,
		"collections", len(tree.Collections), "requests", len(tree.Requests))

	root := ui.NewRoot(paths, store, tree, env)
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "Api-Z",
			Width:     1440,
			Height:    900,
			MinWidth:  900,
			MinHeight: 600,
			Content:   mygoui.View(root.View),
		})
		root.Attach(win)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
