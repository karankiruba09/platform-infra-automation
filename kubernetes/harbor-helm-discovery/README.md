# Harbor OCI Helm discovery

This helper lists OCI Helm charts from Harbor and writes the results to local files.

It is read-only against Harbor:

- it uses Harbor `GET` API calls
- it does not pull charts
- it does not modify Harbor content

## Usage

```sh
HARBOR_URL=https://harbor.example.com HARBOR_USER=admin HARBOR_PASSWORD='secret' HARBOR_INSECURE=true \
./kubernetes/harbor-helm-discovery/list-harbor-helm-charts.sh
```

## Output

The script writes:

- a CSV with `project,repository,version`
- a separate totals file next to the CSV

It also prints per-project progress and a final total to the console.

Generated files under `output/` are ignored by git.
