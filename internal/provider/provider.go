package provider

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/akselleirv/sealedsecret/internal/k8s"
	"github.com/akselleirv/sealedsecret/internal/kubeseal"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"os"
)

func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"kubernetes": {
				Type:        schema.TypeList,
				MaxItems:    1,
				Required:    true,
				Description: "Kubernetes configuration.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"host": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The hostname (in form of URI) of Kubernetes master.",
							DefaultFunc: schema.EnvDefaultFunc("HOST", nil),
						},
						"client_certificate": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "PEM-encoded client certificate for TLS authentication.",
							DefaultFunc: envDefaultFuncDecodeBase64("CLIENT_CERTIFICATE", nil),
						},
						"client_key": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "PEM-encoded client certificate key for TLS authentication.",
							DefaultFunc: envDefaultFuncDecodeBase64("CLIENT_KEY", nil),
						},
						"cluster_ca_certificate": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "PEM-encoded root certificates bundle for TLS authentication.",
							DefaultFunc: envDefaultFuncDecodeBase64("CLUSTER_CA_CERTIFICATE", nil),
						},
					},
				},
			},
			"controller_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The name of k8s service for the sealed-secret-controller.",
				Default:     "sealed-secret-controller-sealed-secrets",
			},
			"controller_namespace": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The namespace the controller is running in.",
				Default:     "kube-system",
			},
		},
		ConfigureContextFunc: configureProvider,
		ResourcesMap: map[string]*schema.Resource{
			"sealedsecret_local": resourceLocal(),
		},
	}
}

type ProviderConfig struct {
	ControllerName      string
	ControllerNamespace string
	Client              *k8s.Client
	PublicKeyResolver   kubeseal.PKResolverFunc
}

func configureProvider(ctx context.Context, rd *schema.ResourceData) (interface{}, diag.Diagnostics) {
	k8sCfg, ok := getMapFromSchemaSet(rd, "kubernetes")
	if !ok {
		return nil, diag.FromErr(errors.New("k8s configuration is required"))
	}
	c, err := k8s.NewClient(&k8s.Config{
		Host:          k8sCfg["host"].(string),
		ClusterCACert: []byte(k8sCfg["cluster_ca_certificate"].(string)),
		ClientCert:    []byte(k8sCfg["client_certificate"].(string)),
		ClientKey:     []byte(k8sCfg["client_key"].(string)),
	})
	if err != nil {
		return nil, diag.FromErr(err)
	}

	cName := rd.Get("controller_name").(string)
	cNs := rd.Get("controller_namespace").(string)

	return &ProviderConfig{
		ControllerName:      cName,
		ControllerNamespace: cNs,
		Client:              c,
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

func envDefaultFuncDecodeBase64(k string, dv interface{}) schema.SchemaDefaultFunc {
	return func() (interface{}, error) {
		if v := os.Getenv(k); v != "" {
			devV, err := base64.StdEncoding.DecodeString(v)
			if err != nil {
				return nil, fmt.Errorf("unable to decode default key %s: %w", k, err)
			}
			return string(devV), nil
		}

		return dv, nil
	}
}
