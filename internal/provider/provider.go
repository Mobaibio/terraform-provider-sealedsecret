package provider

import (
	"context"
	"errors"
	"github.com/akselleirv/sealedsecret/internal/git"
	"github.com/akselleirv/sealedsecret/internal/k8s"
	"github.com/akselleirv/sealedsecret/internal/kubeseal"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	kubernetes           = "kubernetes"
	host                 = "host"
	clientCertificate    = "client_certificate"
	clientKey            = "client_key"
	clusterCaCertificate = "cluster_ca_certificate"
	controllerName       = "controller_name"
	controllerNamespace  = "controller_namespace"
	gitStr               = "git"
	gitlabStr            = "gitlab"
	sealedSecretInGit    = "sealedsecret_in_git"
)

func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			kubernetes: {
				Type:        schema.TypeList,
				MaxItems:    1,
				Required:    true,
				Description: "Kubernetes configuration.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						host: {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The hostname (in form of URI) of Kubernetes master.",
						},
						clientCertificate: {
							Type:        schema.TypeString,
							Required:    true,
							Description: "PEM-encoded client certificate for TLS authentication.",
						},
						clientKey: {
							Type:        schema.TypeString,
							Required:    true,
							Description: "PEM-encoded client certificate key for TLS authentication.",
						},
						clusterCaCertificate: {
							Type:        schema.TypeString,
							Required:    true,
							Description: "PEM-encoded root certificates bundle for TLS authentication.",
						},
					},
				},
			},
			gitStr: {
				Type:        schema.TypeList,
				MaxItems:    1,
				Optional:    true,
				Description: "Git repository credentials to where the sealed secret should be stored.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						url: {
							Type:        schema.TypeString,
							Required:    true,
							Description: "URL to the repository.",
						},
						username: {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Username to be used for the basic auth.",
						},
						token: {
							Type:        schema.TypeString,
							Required:    true,
							Sensitive:   true,
							Description: "Token to be used for the basic auth.",
						},
						sourceBranch: {
							Type:        schema.TypeString,
							Default:     "main",
							Optional:    true,
							Description: "Name of the branch to be used. If the branch does not exist it will be created.",
						},
						targetBranch: {
							Type:        schema.TypeString,
							Default:     "main",
							Optional:    true,
							Description: "Name of the branch that should be merged to. Gitlab value must be set to true in order to create a merge request.",
						},
						gitlabStr: {
							Type:        schema.TypeBool,
							Default:     false,
							Optional:    true,
							Description: "If set to true the provider will create a merge request from source branch to target branch. This is currently supported for Gitlab.",
						},
					},
				},
			},
			controllerName: {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The name of the sealed-secret-controller.",
				Default:     "sealed-data-controller",
			},
			controllerNamespace: {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The namespace the controller is running in.",
				Default:     "kube-system",
			},
		},
		ConfigureContextFunc: configureProvider,
		ResourcesMap: map[string]*schema.Resource{
			sealedSecretInGit:    resourceInGit(),
			"sealedsecret_local": resourceLocal(),
		},
	}
}

type ProviderConfig struct {
	ControllerName      string
	ControllerNamespace string
	Client              *k8s.Client
	Git                 *git.Git
	IsGitlabRepo        bool
	PublicKeyResolver   kubeseal.PKResolverFunc
}

func configureProvider(ctx context.Context, rd *schema.ResourceData) (interface{}, diag.Diagnostics) {
	k8sCfg, ok := getMapFromSchemaSet(rd, kubernetes)
	if !ok {
		return nil, diag.FromErr(errors.New("k8s configuration is required"))
	}
	gitCfg, ok := getMapFromSchemaSet(rd, gitStr)
	var g *git.Git
	var isGitlab bool
	if ok {
		var err error
		g, err = git.NewGit(ctx, gitCfg[url].(string), gitCfg[sourceBranch].(string), gitCfg[targetBranch].(string), git.BasicAuth{
			Username: gitCfg[username].(string),
			Token:    gitCfg[token].(string),
		})
		if err != nil {
			return nil, diag.FromErr(err)
		}
		isGitlab = gitCfg[gitlabStr].(bool)
	} else {
		logDebug("skipping setting up git client since no config was provided")
	}

	c, err := k8s.NewClient(&k8s.Config{
		Host:          k8sCfg[host].(string),
		ClusterCACert: []byte(k8sCfg[clusterCaCertificate].(string)),
		ClientCert:    []byte(k8sCfg[clientCertificate].(string)),
		ClientKey:     []byte(k8sCfg[clientKey].(string)),
	})
	if err != nil {
		return nil, diag.FromErr(err)
	}

	cName := rd.Get(controllerName).(string)
	cNs := rd.Get(controllerNamespace).(string)

	return &ProviderConfig{
		ControllerName:      cName,
		ControllerNamespace: cNs,
		Client:              c,
		Git:                 g,
		IsGitlabRepo:        isGitlab,
		PublicKeyResolver:   kubeseal.FetchPK(c, cName, cNs),
	}, nil
}

func getMapFromSchemaSet(rd *schema.ResourceData, key string) (map[string]interface{}, bool) {
	m, ok := rd.GetOk(key)
	if !ok {
		return nil, ok
	}
	return m.([]interface{})[0].(map[string]interface{}), ok
}
