# aiproxy on Azure Container Apps

Deploys `ghcr.io/egose/aiproxy` (0.25 CPU / 0.5Gi) with the HCL in
`files/aiproxy.hcl` mounted to `/etc/aiproxy/config.hcl` and the client
bearer token generated via `random_password`.

## Prerequisites

- Azure CLI + Terraform `>= 1.11.4`
- Backend storage in `config.tf` (`jdev-azure` / `jdevazureterraform`) must exist

## Run

```sh
az login
SUBSCRIPTION_ID=$(az account show --query id -o tsv)

terraform init
terraform plan -var subscription_id="$SUBSCRIPTION_ID"
terraform apply -var subscription_id="$SUBSCRIPTION_ID"
```

Avoid cold starts / pin a version:

```sh
terraform apply \
  -var subscription_id="$SUBSCRIPTION_ID" \
  -var min_replicas=1 \
  -var image_tag=latest
```

## Verify

```sh
FQDN=$(terraform output -raw fqdn)
TOKEN=$(terraform output -raw client_token)
curl -H "Authorization: Bearer $TOKEN" "https://$FQDN/v1/models"
```

## Config

Edit `files/aiproxy.hcl`, then `terraform apply` (stored as the
`aiproxy-config` secret). The token stays in `env("AIPROXY_CLIENT_TOKEN")`;
rotate with `terraform taint random_password.api_client_token && terraform apply`.

## Cleanup

```sh
terraform destroy -var subscription_id="$SUBSCRIPTION_ID"
```
