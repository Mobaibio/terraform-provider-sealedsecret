package provider

import (
	"context"
	"crypto/rsa"
	"crypto/sha1"
	"fmt"
	"github.com/akselleirv/sealedsecret/internal/k8s"
	"github.com/akselleirv/sealedsecret/internal/kubeseal"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	v1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"log"
	"time"
)

func resourceLocal() *schema.Resource {
	return &schema.Resource{
		Description:   "Creates a sealed secret and store it in yaml_content.",
		ReadContext:   dataSourceLocalRead,
		UpdateContext: dataSourceLocalRead,
		DeleteContext: dataSourceLocalRead,
		CreateContext: dataSourceLocalRead,
		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the secret, must be unique.",
			},
			"namespace": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Namespace of the secret.",
			},
			"type": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "Opaque",
				Description: "The secret type (ex. Opaque). Default type is Opaque.",
			},
			"data": {
				Type:        schema.TypeMap,
				Optional:    true,
				Sensitive:   true,
				Description: "Key/value pairs to populate the secret. The value will be base64 encoded",
			},
			"yaml_content": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The produced sealed secret yaml file.",
			},
			"public_key_hash": {
				Type:        schema.TypeString,
				Computed:    true,
				ForceNew:    true,
				Description: "The public key hashed to detect if the public key changes.",
			},
		},
	}
}

func dataSourceLocalRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	provider := meta.(*ProviderConfig)
	filePath := d.Get("name").(string)

	logDebug("Creating sealed secret for path " + filePath)
	k8sSecret, err := createK8sSecret(d)
	if err != nil {
		return diag.FromErr(err)
	}
	pk, err := fetchPublicKey(ctx, provider.PublicKeyResolver)
	if err != nil {
		return diag.FromErr(err)
	}
	sealedSecret, err := kubeseal.SealSecret(k8sSecret, pk)
	if err != nil {
		return diag.FromErr(err)
	}

	logDebug("Successfully created sealed secret for path " + filePath)
	d.SetId(filePath)
	d.Set("data", d.Get("data").(map[string]interface{}))
	d.Set("yaml_content", string(sealedSecret))
	d.Set("public_key_hash", hashPublicKey(pk))

	return nil
}

func createK8sSecret(d *schema.ResourceData) (v1.Secret, error) {
	rawSecret := k8s.SecretManifest{
		Name:      d.Get("name").(string),
		Namespace: d.Get("namespace").(string),
		Type:      d.Get("type").(string),
	}
	if dataRaw, ok := d.GetOk("data"); ok {
		rawSecret.Data = dataRaw.(map[string]interface{})
	}

	return k8s.CreateSecret(&rawSecret)
}

func fetchPublicKey(ctx context.Context, pkResolver kubeseal.PKResolverFunc) (*rsa.PublicKey, error) {
	var pk *rsa.PublicKey
	err := resource.RetryContext(ctx, 1*time.Minute, func() *resource.RetryError {
		var err error
		logDebug("Trying to fetch the public key")
		pk, err = pkResolver(ctx)
		if err != nil {
			if k8sErrors.IsNotFound(err) || k8sErrors.IsServiceUnavailable(err) {
				logDebug("Retrying to fetch the public key: " + err.Error())
				return resource.RetryableError(fmt.Errorf("waiting for sealed-secret-controller to be deployed: %w", err))
			}
			return resource.NonRetryableError(err)
		}
		logDebug("Successfully fetched the public key")
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pk, nil
}

func hashPublicKey(pk *rsa.PublicKey) string {
	return fmt.Sprintf("%x", sha1.Sum([]byte(fmt.Sprintf("%v%v", pk.N, pk.E))))
}

func logDebug(s string) {
	log.Printf("[DEBUG] %s", s)
}
