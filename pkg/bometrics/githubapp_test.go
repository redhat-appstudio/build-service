package bometrics

import (
	"context"
	"errors"
	"github.com/google/go-github/v45/github"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"strconv"
	"strings"
	"testing"
)

func TestGithubAppAvailability(t *testing.T) {
	tests := []struct {
		name                    string
		client                  client.Client
		getGithubAppCredentials func(ctx context.Context, client client.Client) (int64, []byte, error)
		getGithubApp            func(ctx context.Context, tr http.RoundTripper, appID int64, privateKey []byte) (*github.App, *github.Response, error)
		expected                int
	}{
		{name: "should be able available if no error",
			client:                  fake.NewClientBuilder().Build(),
			getGithubAppCredentials: func(ctx context.Context, client client.Client) (int64, []byte, error) { return 0, nil, nil },
			getGithubApp: func(ctx context.Context, tr http.RoundTripper, appID int64, privateKey []byte) (*github.App, *github.Response, error) {
				return nil, nil, nil
			},
			expected: 1,
		},
		{name: "should not be able available if error on get github app credentials",
			client: fake.NewClientBuilder().Build(),
			getGithubAppCredentials: func(ctx context.Context, client client.Client) (int64, []byte, error) {
				return 0, nil, errors.New("some error")
			},
			getGithubApp: func(ctx context.Context, tr http.RoundTripper, appID int64, privateKey []byte) (*github.App, *github.Response, error) {
				return nil, nil, nil
			},
			expected: 0,
		},
		{name: "should not be able available if error on get github app information",
			client: fake.NewClientBuilder().Build(),
			getGithubAppCredentials: func(ctx context.Context, client client.Client) (int64, []byte, error) {
				return 0, nil, nil
			},
			getGithubApp: func(ctx context.Context, tr http.RoundTripper, appID int64, privateKey []byte) (*github.App, *github.Response, error) {
				return nil, nil, errors.New("some error")
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := &GithubAppAvailabilityProbe{
				client: tt.client,
				gauge: prometheus.NewGauge(
					prometheus.GaugeOpts{
						Namespace: MetricsNamespace,
						Subsystem: MetricsSubsystem,
						Name:      "global_github_app_available",
						Help:      "The availability of the Github App",
					}),
				getGithubAppCredentials: tt.getGithubAppCredentials,
				getGithubApp:            tt.getGithubApp,
			}
			buildMetrics := NewBuildMetrics([]AvailabilityProbe{probe})
			registry := prometheus.NewPedanticRegistry()
			err := buildMetrics.InitMetrics(registry)
			if err != nil {
				t.Errorf("Fail to register metrics: %v", err)
			}

			buildMetrics.checkProbes(context.Background())

			expectedMetrics := `
			# HELP redhat_appstudio_buildservice_global_github_app_available The availability of the Github App
			# TYPE redhat_appstudio_buildservice_global_github_app_available gauge
			redhat_appstudio_buildservice_global_github_app_available `
			expectedMetrics += strconv.Itoa(tt.expected) + "\n"
			err = testutil.GatherAndCompare(registry, strings.NewReader(expectedMetrics), "redhat_appstudio_buildservice_global_github_app_available")
			if err != nil {
				t.Errorf("Fail to gather metrics: %v", err)
			}

		})
	}
}
