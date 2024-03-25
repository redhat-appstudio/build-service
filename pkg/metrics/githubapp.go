package metrics

import (
	"context"
	"fmt"
	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v45/github"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redhat-appstudio/application-service/gitops"
	gitopsprepare "github.com/redhat-appstudio/application-service/gitops/prepare"
	"github.com/redhat-appstudio/build-service/pkg/boerrors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"
)

type GithubAppAvailabilityProbe struct {
	client client.Client
	gauge  prometheus.Gauge
}

func NewGithubAppAvailabilityProbe(client client.Client) *GithubAppAvailabilityProbe {
	return &GithubAppAvailabilityProbe{
		client: client,
		gauge: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: MetricsNamespace,
				Subsystem: MetricsSubsystem,
				Name:      "global_github_app_available",
				Help:      "The availability of the Github App",
			}),
	}
}

func (g *GithubAppAvailabilityProbe) CheckAvailability(ctx context.Context) error {
	log := ctrllog.FromContext(ctx).V(1)
	pacSecret := corev1.Secret{}
	globalPaCSecretKey := types.NamespacedName{Namespace: BuildServiceNamespaceName, Name: gitopsprepare.PipelinesAsCodeSecretName}
	log.Info("Checking GitHub App availability")
	if err := g.client.Get(ctx, globalPaCSecretKey, &pacSecret); err != nil {
		return boerrors.NewBuildOpError(boerrors.EPaCSecretNotFound,
			fmt.Errorf(" Pipelines as Code secret not found in %s namespace nor in %s", BuildServiceNamespaceName, globalPaCSecretKey.Namespace))
	}
	config := pacSecret.Data
	githubAppIdStr := string(config[gitops.PipelinesAsCode_githubAppIdKey])
	githubAppId, err := strconv.ParseInt(githubAppIdStr, 10, 64)
	if err != nil {
		return boerrors.NewBuildOpError(boerrors.EGitHubAppMalformedId,
			fmt.Errorf("failed to create git client: failed to convert %s to int: %w", githubAppIdStr, err))
	}
	log.Info("GitHub App ID", "githubAppId", githubAppId)
	privateKey := config[gitops.PipelinesAsCode_githubPrivateKey]
	if len(config[gitops.PipelinesAsCode_githubPrivateKey]) == 0 {
		return boerrors.NewBuildOpError(boerrors.EPaCSecretInvalid,
			fmt.Errorf("invalid configuration in Pipelines as Code secret"))

	}
	itr, err := ghinstallation.NewAppsTransport(http.DefaultTransport, githubAppId, privateKey)
	if err != nil {
		// Inability to create transport based on a private key indicates that the key is bad formatted
		return boerrors.NewBuildOpError(boerrors.EGitHubAppMalformedPrivateKey, err)
	}
	client := github.NewClient(&http.Client{Transport: itr})
	app, resp, err := client.Apps.Get(ctx, "")
	log.Info("GitHub App", "app", app, "resp", resp, "err", err)
	if err != nil {
		return err
	}
	return nil
}

func (g *GithubAppAvailabilityProbe) AvailabilityGauge() prometheus.Gauge {
	return g.gauge
}
