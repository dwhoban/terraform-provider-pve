# Examples

This directory contains examples that are mostly used for documentation, but can also be run/tested manually via the Terraform CLI.

The document generation tool looks for files in the following locations by default. All other *.tf files besides the ones mentioned below are ignored by the documentation tool. This is useful for creating examples that can run and/or are testable even if some parts are not relevant for the documentation.

* **provider/provider.tf** example file for the provider index page
* **data-sources/`full data source name`/data-source.tf** example file for the named data source page
* **resources/`full resource name`/resource.tf** example file for the named resource page
* **actions/`full action name`/action.tf** example file for the named action page
* **functions/`function name`/function.tf** example file for the named function page

Every example must pass `terraform validate` against the real provider
schema. Validate locally with dev overrides: build the provider as
`terraform-provider-pve`, point a CLI config `dev_overrides` block at the
binary's directory, and run `terraform -chdir=examples/<kind>/<name>
validate` (`examples/functions/pve_next_id` additionally needs configured
provider credentials because the function queries the cluster).
