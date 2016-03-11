package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"syscall"

	"net/http"
	_ "net/http/pprof"

	"github.com/op/go-logging"
	"gopkg.in/yaml.v2"

	"github.com/ditrace/ditrace/dtrace"
	"github.com/ditrace/ditrace/metrics"
)

var (
	log            *logging.Logger
	configLocation = flag.String("config", "config.yml", "path to config file")
	port           = flag.Int64("port", 8080, "http server port")
	printVersion   = flag.Bool("version", false, "Print current version and exit")

	version = "lastest"
)

type profiling struct {
	Enable bool `yaml:"enable"`
}

type config struct {
	LogDir    string          `yaml:"log_dir"`
	LogLevel  string          `yaml:"log_level"`
	Stats     *metrics.Config `yaml:"stats"`
	Profiling *profiling      `yaml:"profiling"`
	HTTP      *dtrace.Config  `yaml:"http"`
}

func getConfig(configLocation string) (*config, error) {
	config := &config{
		LogDir:   "stdout",
		LogLevel: "debug",
		HTTP: &dtrace.Config{
			Enable:            true,
			Address:           ":8080",
			MaxActiveRequests: 100,
			BulkSize:          1000,
			Sampling:          1,
			Replicas:          []string{"http://localhost:8080"},
			MyReplicaIndex:    0,
			MinTTL:            10,
			MaxTTL:            120,
		},
	}

	configYaml, err := ioutil.ReadFile(configLocation)
	if err != nil {
		return nil, fmt.Errorf("Can't read file [%s] [%s]", configLocation, err)
	}
	err = yaml.Unmarshal([]byte(configYaml), &config)
	if err != nil {
		return nil, fmt.Errorf("Can't parse config file [%s] [%s]", configLocation, err)
	}
	return config, nil
}

func newLog(module, logDir, filename, level string) *logging.Logger {
	logLevel, err := logging.LogLevel(level)
	if err != nil {
		logLevel = logging.DEBUG
	}
	var logBackend *logging.LogBackend
	if logDir == "stdout" || logDir == "" {
		logBackend = logging.NewLogBackend(os.Stdout, "", 0)
		logBackend.Color = true
	} else {
		if err := os.MkdirAll(logDir, 755); err != nil {
			fmt.Printf("Can't create log directories %s: %s", logDir, err.Error())
			os.Exit(1)
		}
		logFileName := path.Join(logDir, filename)
		logFile, err := os.OpenFile(logFileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			fmt.Printf("Can't open log file %s: %s", logFileName, err.Error())
			os.Exit(1)
		}
		logBackend = logging.NewLogBackend(logFile, "", 0)
	}
	logger := logging.MustGetLogger(module)
	leveledLogBackend := logging.AddModuleLevel(logBackend)
	leveledLogBackend.SetLevel(logLevel, module)
	logger.SetBackend(leveledLogBackend)
	return logger
}

func main() {
	flag.Parse()
	if *printVersion {
		fmt.Println("go-dtrace. Version: ", version)
		os.Exit(0)
	}

	terminate := make(chan os.Signal)
	signal.Notify(terminate, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	logging.SetFormatter(logging.MustStringFormatter("%{time:2006-01-02 15:04:05}\t%{level}\t%{message}"))

	config, err := getConfig(*configLocation)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	log = newLog("http", config.LogDir, "http.log", config.LogLevel)
	metrics.SetLogger(newLog("metrics", config.LogDir, "metrics.log", config.LogLevel))
	dtrace.SetLogger(log)

	err = metrics.InitStats(config.Stats)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if config.Profiling.Enable {
		go func() {
			log.Info("Starting profiling server")
			if err := http.ListenAndServe("localhost:6060", nil); err != nil {
				log.Warningf("Error starting profiling: %s", err)
			}
		}()
	}

	dtrace.Listen(terminate, config.HTTP, make(chan bool, 1))
	log.Info("Stopped")
}
