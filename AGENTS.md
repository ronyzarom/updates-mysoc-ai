# Updater rollout policy

- Testing servers, including pod A, B, and witness, use the normal updater release channel (`stable`) with the `alpha` fleet group and automatic signed self-updates. Pod topology does not require a separate updater release channel.
- Do not create or preserve special updater channels, manual binary delivery, or per-host updater pins for testing unless the user explicitly requests an exception. Check effective local self-update channel as well as the central ring before claiming alpha rollout is configured.
- Preserve explicit user holds (including the benchmark hold) until the user removes them. Product release channels, product upgrade prerequisites, and application health/rollback requirements remain separate from updater self-update settings.
- Verify actual installed updater version, successful restart, cascade heartbeat, and application health after rollout. Publication alone is not deployment.
