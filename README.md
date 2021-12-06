# sealed-secrets-to-git

# What is this provider doing?

* Fetches the sealed secret controller's public key.
* Encrypts the provided secret.

The sealed secret manifest is computed on the `yaml_content` field.