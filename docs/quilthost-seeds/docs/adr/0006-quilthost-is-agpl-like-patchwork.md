# Quilthost is AGPL-3.0, like Patchwork

The seamrip guarantee commits Quilthost to "the same license posture as
Patchwork," and Patchwork is AGPL-3.0 — so Quilthost is too. AGPL is the
coherent choice for an anti-lock-in host: a competitor may run the
control plane as a service, and must publish their modifications if they
do. No license contagion concern in either direction: the control plane
runs Patchwork's binary as a separate program, it links none of its code.
The income is for operating the service, never for secret code.
