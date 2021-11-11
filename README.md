# sealed-secrets-to-git

# What is this provider doing?

1. Get public key from sealed-secret-controller.
2. Encrypt the provided secret manifest.
3. Push to Git.

Then the sealed secret is picked up by for example Flux CD.
If the public key is changed it will be recreate the sealed secrets.
