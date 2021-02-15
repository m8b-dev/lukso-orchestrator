package orchestrator

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"io"
	"os"
	"time"
)

const (
	CatalystImage              = "silesiacoin/ssc20-client:v002"
	TekuImage                  = "consensys/teku:latest"
	CatalystClientName         = "catalyst"
	CatalystContainerName      = "luksoCatalyst"
	LuksoContainerNameSelector = "label"
	TekuClientName             = "teku"
)

var (
	catalystArguments = []string{
		"./geth",
		"--rpc",
		"--rpcapi", "net,eth,eth2,web3,personal,admin,db,debug,miner,shh,txpool",
		"--rpccorsdomain", "*",
		"--rpcaddr", "0.0.0.0",
		"--verbosity", "5",
		"--txpool.processtxs", "--txpool.accountslots", "10000", "--txpool.accountqueue", "20000",
	}

	tekuArguments = []string{
		"./teku/bin/teku",
		"--network=minimal",
		"--p2p-enabled=true",
		`--rest-api-host-allowlist="*"`,
		"--rest-api-port=5051",
		"--rest-api-enabled=true",
		"--metrics-enabled=true",
	}
)

type Orchestrator struct {
	params    *Params
	dockerCli *client.Client
}

type Params struct{}

func New(params *Params) (orchestratorClient *Orchestrator, err error) {
	orchestratorClient = &Orchestrator{params: params}
	dockerClient, err := orchestratorClient.newDockerClient()
	orchestratorClient.dockerCli = dockerClient

	return
}

func (orchestratorClient *Orchestrator) SpinEth1(clientName string) (
	containerBody container.ContainerCreateCreatedBody,
	err error,
) {
	if CatalystClientName != clientName {
		err = fmt.Errorf("client %s not supported, valid %s", clientName, CatalystClientName)

		return
	}

	err = orchestratorClient.isContainerRunningWithImage(CatalystImage)

	if nil != err {
		return
	}

	containerBody, err = orchestratorClient.createCatalystContainer()

	if nil != err {
		return
	}

	err = orchestratorClient.dockerCli.ContainerStart(
		context.Background(),
		containerBody.ID,
		types.ContainerStartOptions{},
	)

	return
}

func (orchestratorClient *Orchestrator) SpinEth2(clientName string) (
	containerBody container.ContainerCreateCreatedBody,
	err error,
) {
	orchestratorClient.assureDockerClient()

	if TekuClientName != clientName {
		err = fmt.Errorf("client %s not supported, valid %s", clientName, TekuClientName)

		return
	}

	err = orchestratorClient.isContainerRunningWithImage(TekuImage)

	if nil != err {
		return
	}

	containerBody, err = orchestratorClient.createTekuContainer()

	if nil != err {
		return
	}

	err = orchestratorClient.dockerCli.ContainerStart(
		context.Background(),
		containerBody.ID,
		types.ContainerStartOptions{},
	)

	return
}

// Run will return err if any of errors happened, and errList with all of the errors that had happened.
// Best to assume that if errList is not empty run was faulty
func (orchestratorClient *Orchestrator) Run(timeout *time.Duration) (
	err error,
	errList []error,
	runningContainers []container.ContainerCreateCreatedBody,
) {
	errList = make([]error, 0)
	runningContainers = make([]container.ContainerCreateCreatedBody, 0)
	err, errList = orchestratorClient.stopTekuCatalystImages(timeout)

	if nil != err {
		return
	}

	eth1Container, err := orchestratorClient.SpinEth1(CatalystClientName)

	if nil != err {
		return
	}

	runningContainers = append(runningContainers, eth1Container)
	eth2Container, err := orchestratorClient.SpinEth2(TekuClientName)

	if nil != err {
		return
	}

	runningContainers = append(runningContainers, eth2Container)

	return
}

func (orchestratorClient *Orchestrator) LogsFromContainers(
	containerList []container.ContainerCreateCreatedBody,
	stopChan chan bool,
) (err error) {
	dockerClient := orchestratorClient.assureDockerClient()

	for _, runningContainer := range containerList {
		go func(stopChan chan bool, runningContainer container.ContainerCreateCreatedBody) {
			ctx := context.Background()
			options := types.ContainerLogsOptions{ShowStdout: true}
			output, err := dockerClient.ContainerLogs(ctx, runningContainer.ID, options)

			if err != nil {
				fmt.Printf("\n Error occured in container: %s, err: %s", runningContainer.ID, err.Error())
			}

			defer func() {
				_ = output.Close()
			}()

			<-stopChan
		}(stopChan, runningContainer)
	}

	select {
	case <-stopChan:
		return
	}
}

func (orchestratorClient *Orchestrator) isContainerRunningWithImage(imageName string) (err error) {
	imageList, err := orchestratorClient.findRunningContainerByImage(imageName)

	if nil != err {
		return
	}

	if len(imageList) > 0 {
		err = fmt.Errorf("container from image %s should not be running in docker", CatalystImage)

		return
	}

	return
}

// For now lets steer from ENV variables,
// TODO: provide documentation of possible env that you can use, and --help in cli
func (orchestratorClient *Orchestrator) newDockerClient() (dockerCli *client.Client, err error) {
	dockerCli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())

	if nil != err {
		return
	}

	// This logic will be removed from here, but right now we have common docker image for teku and catalyst
	// so we pull it once
	ctx := context.Background()
	out, err := dockerCli.ImagePull(ctx, CatalystImage, types.ImagePullOptions{})

	if nil != err {
		return
	}

	_, err = io.Copy(os.Stdout, out)

	return
}

func (orchestratorClient *Orchestrator) assureDockerClient() (dockerCli *client.Client) {
	dockerCli = orchestratorClient.dockerCli

	if nil == dockerCli {
		panic(fmt.Errorf("orchestratorClient only works in docker mode for now, please use New() func"))
	}

	return
}

func (orchestratorClient *Orchestrator) findRunningContainerByImage(imageName string) (
	containerList []types.Container,
	err error,
) {
	ctx := context.Background()
	dockerCli := orchestratorClient.assureDockerClient()
	allContainers, err := dockerCli.ContainerList(ctx, types.ContainerListOptions{})

	if nil != err {
		return
	}

	containerList = make([]types.Container, 0)

	for _, runningContainer := range allContainers {
		if imageName == runningContainer.Image {
			containerList = append(containerList, runningContainer)
		}
	}

	return
}

func (orchestratorClient *Orchestrator) stopContainers(
	containerList []types.Container,
	timeout *time.Duration,
) (err error, errList []error) {
	dockerClient := orchestratorClient.assureDockerClient()
	ctx := context.Background()
	errList = make([]error, 0)

	for _, containerToStop := range containerList {
		latestError := dockerClient.ContainerStop(ctx, containerToStop.ID, timeout)

		if nil != latestError {
			err = latestError
			errList = append(errList, latestError)
		}
	}

	return
}

func (orchestratorClient *Orchestrator) createCatalystContainer() (
	containerBody container.ContainerCreateCreatedBody,
	err error,
) {
	dockerCli := orchestratorClient.assureDockerClient()
	ctx := context.Background()
	containerBody, err = dockerCli.ContainerCreate(
		ctx,
		&container.Config{
			// For now lets try with root
			User:         "root",
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			Tty:          false,
			OpenStdin:    false,
			StdinOnce:    false,
			Env:          nil,
			Cmd:          catalystArguments,
			Image:        CatalystImage,
			Labels:       map[string]string{LuksoContainerNameSelector: CatalystContainerName},
		},
		&container.HostConfig{},
		&network.NetworkingConfig{},
		nil,
		"",
	)

	return
}

func (orchestratorClient *Orchestrator) createTekuContainer() (
	containerBody container.ContainerCreateCreatedBody,
	err error,
) {
	dockerCli := orchestratorClient.assureDockerClient()
	ctx := context.Background()
	containerBody, err = dockerCli.ContainerCreate(
		ctx,
		&container.Config{
			// For now lets try with root
			User:         "root",
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			Tty:          false,
			OpenStdin:    false,
			StdinOnce:    false,
			Env:          nil,
			Cmd:          tekuArguments,
			Image:        CatalystImage,
		},
		&container.HostConfig{},
		&network.NetworkingConfig{},
		nil,
		"",
	)

	return
}

func (orchestratorClient *Orchestrator) stopTekuCatalystImages(timeout *time.Duration) (err error, errList []error) {
	containerList := make([]types.Container, 0)
	catalystContainerList, err := orchestratorClient.findRunningContainerByImage(CatalystImage)

	if nil != err {
		return
	}

	tekuContainerList, err := orchestratorClient.findRunningContainerByImage(TekuClientName)

	if nil != err {
		return
	}

	containerList = append(containerList, catalystContainerList...)
	containerList = append(containerList, tekuContainerList...)

	err, errList = orchestratorClient.stopContainers(catalystContainerList, timeout)

	return
}
