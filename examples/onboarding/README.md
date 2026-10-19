# Domain onboarding (mock-tested alpha)

The provider is unpublished and all MXroute runtime behavior is live-unverified.
Use a local provider development override. `main.tf` illustrates Route 53; adapt
its reserved example domain to an owned disposable domain in private working
configuration outside this repository. Supply zone IDs and credentials at runtime.
It is an example, not authorization to run live account changes.

The dependency chain is verification key → external DNS TXT → bounded DNS
propagation check → MXroute domain → mail DNS discovery → MX/SPF publication.
The optional wait polls your configured system DNS resolver for the exact TXT
record. A successful DNS-provider apply alone does not establish propagation.
The upstream API can use a different resolver; a rejected API create is reported
without retrying arbitrary validation errors. Resolve that condition before a
later apply. Disable preflight with `verification_wait_seconds = 0` when DNS is
handled separately. Visibility polling retries only confirmed absence, up to
`visibility_wait_seconds`; other errors stop immediately. Both controls accept
0–1800 seconds.

DKIM and verification outputs may be null. The full DKIM object is provided as
an output so it can be consumed in a separate DNS configuration or staged apply
without using an unknown nullable value as a first-plan `for_each` key. The DNS
API's name and content are preserved verbatim; adapt relative names to your DNS
provider's zone format and handle its TXT length rules if required.

A pointer's parent must already exist. Reference `mxroute_domain.example.domain`
from the pointer resource so Terraform deletes the pointer before the parent.
Verification of the parent does not verify the pointer's DNS name. Optional
pointer preflight checks its own name; publish its TXT separately before enabling
that wait. Exact upstream pointer prerequisites are still live-unverified. Alias,
redirect, parent and name changes require replacement. Unique pointer names may
prevent create-before-destroy.

Domain deletion consequences and recovery are live-unverified. It can affect
mail and child objects. Keep `prevent_destroy` on important domains. The provider
never deletes unmanaged mailboxes or pointers to make deletion pass. Explicit
child resources must be removed before their parent; unresolved server errors
preserve state and stop cleanup.

If domain creation succeeds but visibility or mail-hosting setup fails, Terraform
receives a warning and saves `mail_hosting_pending = true`. In that pending state,
`mail_hosting` retains requested intent until refresh and must not be read as
proof of the remote setting. Review the warning, then run a normal refreshed
plan/apply to read the actual setting and finish setup. Avoid `-refresh=false`
when recovering. This prevents Terraform tainting and replacing the new domain.
Import an existing domain with `terraform import mxroute_domain.example
example.com`; import a pointer with `terraform import mxroute_domain_pointer.example
example.com/alias.example.net`. Configure its current mode and hosting value,
then confirm a no-change plan before applying any changes.

The automated onboarding test uses only local mock HTTP and DNS responses. It
checks the same key/verification/create/discovery sequence without executing
Route 53 writes. No live DNS or MXroute operation is tested or authorized.

The external DNS example follows the [AWS provider Route 53 record contract](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/route53_record).
