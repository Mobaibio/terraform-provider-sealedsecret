package provider

import (
	"context"
	"crypto/rsa"
	"crypto/sha1"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/akselleirv/sealedsecret/internal/k8s"
	"github.com/akselleirv/sealedsecret/internal/kubeseal"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	name          = "name"
	namespace     = "namespace"
	secretType    = "type"
	data          = "data"
	stringData    = "string_data"
	filepath      = "filepath"
	publicKeyHash = "public_key_hash"
)
const (
	username     = "username"
	token        = "token"
	url          = "url"
	sourceBranch = "source_branch"
	targetBranch = "target_branch"
)

type SealedSecret struct {
	Spec struct {
		EncryptedData map[string]string `yaml:"encryptedData"`
		Template      struct {
			Type     string `yaml:"type"`
			Metadata struct {
				Name      string `yaml:"name"`
				Namespace string `yaml:"namespace"`
			} `yaml:"metadata"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

func resourceInGit() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceCreateInGit,
		ReadContext:   resourceReadInGit,
		UpdateContext: resourceUpdateInGit,
		DeleteContext: resourceDeleteInGit,
		Schema: map[string]*schema.Schema{
			name: {
				Type:        schema.TypeString,
				Required:    true,
				Description: "name of the secret, must be unique",
			},
			namespace: {
				Type:        schema.TypeString,
				Required:    true,
				Description: "namespace of the secret",
			},
			secretType: {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "Opaque",
				Description: "The secret type (ex. Opaque)",
			},
			data: {
				Type:        schema.TypeMap,
				Optional:    true,
				Sensitive:   true,
				Description: "Key/value pairs to populate the secret. The value will be base64 encoded",
			},
			stringData: {
				Type:        schema.TypeMap,
				Optional:    true,
				Sensitive:   true,
				Description: "Key/value pairs to populate the secret.",
			},
			filepath: {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The filepath in the Git repository. Including the filename itself and extension",
			},
			publicKeyHash: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The public key hashed to detect if the public key changes.",
			},
		},
	}
}

func resourceCreateInGit(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	provider := meta.(*ProviderConfig)
	filePath := d.Get(filepath).(string)

	logDebug("Creating sealed secret for path " + filePath)
	sealedSecret, err := createSealedSecret(ctx, provider, d)
	if err != nil {
		return diag.FromErr(err)
	}
	logDebug("Successfully created sealed secret for path " + filePath)

	logDebug("Pushing sealed secret for " + filePath)
	err = provider.Git.Push(ctx, sealedSecret, filePath)
	if err != nil {
		return diag.FromErr(err)
	}
	logDebug("Successfully pushed sealed secret for " + filePath)
	if provider.IsGitlabRepo {
		logDebug("Creating merge request")
		if err = provider.Git.CreateMergeRequest(); err != nil {
			return diag.FromErr(err)
		}
		logDebug("Successfully created merge request")
	}
	d.SetId(filePath)
	if err := d.Set(data, d.Get(data).(map[string]interface{})); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set(stringData, d.Get(stringData).(map[string]interface{})); err != nil {
		return diag.FromErr(err)
	}

	return resourceReadInGit(ctx, d, meta)
}
func resourceReadInGit(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	provider := meta.(*ProviderConfig)

	f, err := provider.Git.GetFile(d.Id())
	if errors.Is(err, os.ErrNotExist) {
		d.SetId("")
		return nil
	}

	if err != nil {
		return diag.FromErr(err)
	}

	ssInGit := &SealedSecret{}
	if err := yaml.Unmarshal(f, ssInGit); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set(name, ssInGit.Spec.Template.Metadata.Name); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set(namespace, ssInGit.Spec.Template.Metadata.Namespace); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set(secretType, ssInGit.Spec.Template.Type); err != nil {
		return diag.FromErr(err)
	}

	pk, err := provider.PublicKeyResolver(ctx)
	if err != nil {
		return diag.FromErr(err)
	}
	newPkHash := hashPublicKey(pk)
	oldPkHash, ok := d.State().Attributes[publicKeyHash]
	if ok && newPkHash != oldPkHash {
		// If the PK changed then we are forcing it to be recreated.
		// We do not require any clean up since the keys stored in Git will be overwritten when applied again.
		// An improvement could be so notify the user the reason for the recreate was the PK change.
		d.SetId("")
	}

	if err := d.Set(publicKeyHash, newPkHash); err != nil {
		return diag.FromErr(err)
	}

	return nil
}
func resourceUpdateInGit(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return resourceCreateInGit(ctx, d, meta)
}
func resourceDeleteInGit(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	provider := meta.(*ProviderConfig)

	err := provider.Git.DeleteFile(ctx, d.Get(filepath).(string))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return diag.FromErr(err)
	}

	if provider.IsGitlabRepo {
		return diag.FromErr(provider.Git.CreateMergeRequest())
	}

	d.SetId("")

	return nil
}

func createSealedSecret(ctx context.Context, provider *ProviderConfig, d *schema.ResourceData) ([]byte, error) {
	rawSecret := k8s.SecretManifest{
		Name:      d.Get(name).(string),
		Namespace: d.Get(namespace).(string),
		Type:      d.Get(secretType).(string),
	}
	if dataRaw, ok := d.GetOk(data); ok {
		rawSecret.Data = dataRaw.(map[string]interface{})
	}
	if stringDataRaw, ok := d.GetOk(stringData); ok {
		m := make(map[string]string)
		for k, v := range stringDataRaw.(map[string]interface{}) {
			m[k] = v.(string)
		}
		rawSecret.StringData = m
	}

	secret, err := k8s.CreateSecret(&rawSecret)
	if err != nil {
		return nil, err
	}

	var pk *rsa.PublicKey
	err = resource.RetryContext(ctx, 3*time.Minute, func() *resource.RetryError {
		var err error
		logDebug("Trying to fetch the public key")
		pk, err = provider.PublicKeyResolver(ctx)
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

	return kubeseal.SealSecret(secret, pk)
}

// The public key is hashed since we want to force update the resource if the key changes.
// Hashing the key also saves us some space.
func hashPublicKey(pk *rsa.PublicKey) string {
	return fmt.Sprintf("%x", sha1.Sum([]byte(fmt.Sprintf("%v%v", pk.N, pk.E))))
}

func logDebug(msg string) {
	log.Printf("[DEBUG] %s\n", msg)
}
