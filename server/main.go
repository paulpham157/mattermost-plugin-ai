// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/agents"
	"github.com/mattermost/mattermost-plugin-ai/api"
	"github.com/mattermost/mattermost-plugin-ai/metrics"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/shared/httpservice"
)

func main() {
	plugin.ClientMain(&Plugin{})
}

type Plugin struct {
	plugin.MattermostPlugin

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration

	pluginAPI             *pluginapi.Client
	llmUpstreamHTTPClient *http.Client

	agentsService *agents.AgentsService
	apiService    *api.API
}

func (p *Plugin) OnActivate() error {
	p.pluginAPI = pluginapi.NewClient(p.API, p.Driver)

	p.llmUpstreamHTTPClient = httpservice.MakeHTTPServicePlugin(p.API).MakeClient(true)
	p.llmUpstreamHTTPClient.Timeout = time.Minute * 10 // LLM requests can be slow

	untrustedHTTPClient := httpservice.MakeHTTPServicePlugin(p.API).MakeClient(false)

	metricsService := metrics.NewMetrics(metrics.InstanceInfo{
		InstallationID: os.Getenv("MM_CLOUD_INSTALLATION_ID"),
		PluginVersion:  manifest.Version, // Manifest imported from manifest.go which is generated by the build process
	})

	// Initialize the agents service
	agentsService, err := agents.NewAgentsService(p.API, p.pluginAPI, p.llmUpstreamHTTPClient, untrustedHTTPClient, metricsService, &p.configuration.Config)
	if err != nil {
		return err
	}

	p.agentsService = agentsService

	// Initialize the API service
	p.apiService = api.New(agentsService, p.pluginAPI, metricsService)

	return nil
}

func (p *Plugin) OnDeactivate() error {
	if p.agentsService != nil {
		if err := p.agentsService.OnDeactivate(); err != nil {
			p.pluginAPI.Log.Error("Error during AgentsService deactivation", "error", err)
			return err
		}
	}
	return nil
}

func (p *Plugin) MessageHasBeenPosted(c *plugin.Context, post *model.Post) {
	p.agentsService.MessageHasBeenPosted(c, post)
}

func (p *Plugin) MessageHasBeenUpdated(c *plugin.Context, newPost, oldPost *model.Post) {
	p.agentsService.MessageHasBeenUpdated(c, newPost, oldPost)
}

func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.apiService.ServeHTTP(c, w, r)
}

func (p *Plugin) ServeMetrics(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.apiService.ServeMetrics(c, w, r)
}
