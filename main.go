package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/route-registrar/config"
	"github.com/cloudfoundry-incubator/route-registrar/healthchecker"
	"github.com/cloudfoundry-incubator/route-registrar/messagebus"
	"github.com/cloudfoundry-incubator/route-registrar/registrar"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
)

func main() {
	var configPath string
	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	pidfile := flags.String("pidfile", "", "Path to pid file")
	cf_lager.AddFlags(flags)

	flags.StringVar(&configPath, "configPath", "", "path to configuration file with json encoded content")
	flags.Set("configPath", "registrar_settings.yml")

	flags.Parse(os.Args[1:])

	logger, _ := cf_lager.New("Route Registrar")

	logger.Info("Initializing")

	configSchema, err := config.NewConfigSchemaFromFile(configPath)
	if err != nil {
		logger.Fatal("error parsing file: %s\n", err)
	}

	c, err := configSchema.ToConfig()
	if err != nil {
		log.Fatalln(err)
	}

	hc := healthchecker.NewHealthChecker(logger)

	logger.Info("creating nats connection")
	messageBus := messagebus.NewMessageBus(logger)

	r := registrar.NewRegistrar(*c, hc, logger, messageBus)

	if *pidfile != "" {
		pid := strconv.Itoa(os.Getpid())
		err := ioutil.WriteFile(*pidfile, []byte(pid), 0644)
		logger.Info("Writing pid", lager.Data{"pid": pid, "file": *pidfile})
		if err != nil {
			logger.Fatal(
				"error writing pid to pidfile",
				err,
				lager.Data{
					"pid":     pid,
					"pidfile": *pidfile,
				},
			)
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)

	logger.Info("Running")

	process := ifrit.Invoke(r)
	for {
		select {
		case s := <-sigChan:
			logger.Info("Caught signal", lager.Data{"signal": s})
			process.Signal(s)
		case err := <-process.Wait():
			if err != nil {
				logger.Fatal("Exiting with error", err)
			}
			logger.Info("Exiting without error")
			os.Exit(0)
		}
	}
}
