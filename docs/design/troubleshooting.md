# Troubleshooting and compatibility

Phase 7 checked ordinary-account reads and later the isolated creation/update
and Terraform adoption of one newly created test mailbox, followed by its
separately authorized deletion and original-baseline verification. Other resource writes,
reseller access and impersonation remain live-unverified. Preserve current state
when a read fails; a failed read is not proof that an object was deleted.

| Symptom | Action |
| --- | --- |
| 401 or 403 | Check complete API key, server, username, role and key restrictions through the panel. Password authentication requires `api_with_password=yes`; login keys follow their own restrictions. Never paste secrets into logs or issue reports. |
| Empty/malformed response or unexpected fields | Keep state. The strict decoder requires all lifecycle-critical fields; optional prose may be absent and documented nullable values may be null. Report the operation and sanitized error category, without raw responses. |
| Quota protocol mismatch | Phase 7 found standard `success/data` envelopes on quota reads despite raw-object documentation. Both complete forms are supported. Missing data and false/invalid success markers still fail. |
| 404 during refresh | Verify the object in the correct account using a successful complete list. A missing access domain or detail-read error cannot establish shared-entry absence. Never remove state based solely on a 404. |
| Repeated diff after import | Match remote values and units, destination sets and canonical identity. Imported passwords need neither a secret nor a rotation version. Shared spam objects need one owner across domains/states. Review an ordinary plan until empty before permitting changes. |
| Confirmed create followed by failed readback | Terraform may taint the resource. Inspect remote existence and state before any replacement; replacement can delete real mail. Do not blindly apply or clean up. |
| Ambiguous write or timeout | Writes with uncertain outcomes are not blindly replayed. Read and reconcile the object; existence alone cannot verify a password change or mail-delivery side effect. Confirmed password updates record their rotation version before readback. |
| Partial reseller update | After a fresh read, the next apply resumes only remaining steps in the documented order. Do not recreate the user. Package/override precedence remains live-unverified. |
| Rate limiting or busy settings | Allow the bounded backoff to complete or cancel. The client uses 180 reads/48 writes per rolling minute, below the documented limits, with at most three attempts. Shared account budgets and setting locks coordinate aliases in one provider process; separate Terraform processes have independent budgets. |
| Forwarder replacement failure | A successful old-alias deletion followed by failed creation leaves routing absent. Failed deletion preserves it. Review the actual outcome before explicitly repairing routing; there is no automatic rollback. |
| Singleton removed but behavior remains | Catch-all and spam settings have no API delete/reset route. Removing management preserves the remote value. Changing that value is a separate explicit update. |

Every operation has a two-minute deadline, each HTTP attempt has a 30-second
timeout, responses are limited to 4 MiB and reads retry bounded transient failures.
Only HTTP 429 is treated as proof that a write was rejected before processing.
Large collection lookups use a list rather than one request per item; inventories
over the response-size cap fail instead of being treated as truncated success.
The API declares no pagination. Successful live reads do not independently prove
that an upstream inventory was complete.

## Compatibility policy

Terraform >= 1.11.1 and < 2.0 is required; 1.11.0 has a known write-only serialization
failure and is excluded. Phase 7 reruns the full offline suite on the minimum
1.11.1 and current pinned 1.16.6 with Go 1.26.9 and 1.27.1 respectively on macOS
arm64. CI covers Linux and macOS for both versions; local results are not evidence
of an unobserved CI run. Cross-compilation does not demonstrate OS runtime
compatibility. Windows and OpenTofu have no runtime support commitment yet.
Cross-builds passed for macOS, Linux and Windows on amd64 and arm64 after lifting
sandbox restrictions on target packages. These builds do not establish runtime
compatibility on systems where the lifecycle suite has not been executed.

All resources remain schema version 0. The schema compatibility fixture freezes
types, required/optional/computed and secret-persistence flags, excluding prose.
There is no earlier released provider state to upgrade. A future schema version
must add realistic synthetic prior-state fixtures and test the actual migration,
including null, unknown, imported password versions and preservation of identities.
Do not add speculative no-op migrations. Check the migration guide and release
notes before upgrading any preview state. Publication and signed releases remain
Phase 8 work.
