package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceLocal() *schema.Resource {
	return &schema.Resource{
		Description:   "Creates a sealed secret and store it in yaml_content.",
		CreateContext: resourceCreateLocal,
		ReadContext:   resourceCreateLocal,
		UpdateContext: resourceCreateLocal,
		DeleteContext: resourceDeleteLocal,
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
				Description: "The secret type (ex. Opaque). Default type is Opaque.",
			},
			data: {
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
		},
	}
}

func resourceCreateLocal(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	provider := meta.(*ProviderConfig)
	filePath := d.Get(name).(string)

	logDebug("Creating sealed secret for path " + filePath)
	sealedSecret, err := createSealedSecret(ctx, provider, d)
	if err != nil {
		return diag.FromErr(err)
	}
	logDebug("Successfully created sealed secret for path " + filePath)

	d.SetId(filePath)
	d.Set(data, d.Get(data).(map[string]interface{}))
	d.Set("yaml_content", string(sealedSecret))

	return nil
}

func resourceDeleteLocal(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	d.SetId("")
	return nil
}
