# API‑First Identity for a Go PaaS (Headless)

## Summarized Requirements (Rephrased)
- **Platform:** Golang PaaS using Traefik (edge) and Nomad (orchestration); runs on our servers and can attach customer infrastructure.
- **User types:** (1) **Organizations** (enterprises/public sector), (2) **Individuals** (developers).
- **Organizations require:** Team **RBAC**; SSO via **OIDC/SAML/LDAP**; option to keep all auth on **customer‑managed, on‑prem** IdP.
- **Individuals require:** Social login (**Google, GitHub**); ability to share app access with **fine‑grained roles** (modify apps, manage access, run features).
- **Constraints:** **API‑driven/headless** only (PaaS provides all UI/CLI/TUI); **open‑source first** with SaaS allowed for fast start + **migration** path; readiness for **SOC 2 / HIPAA / GDPR**; **multi‑tenant**, auditable, and scalable.

---

## API‑First Candidate Solutions — Comparison

| Solution | Type / Deploy | Pros (API & strengths) | Cons | Best Fit |
|---|---|---|---|---|
| **Ory Kratos (+ Hydra)** | OSS, Go; self‑host or Ory Cloud | **Headless** flows; bring‑your‑own UI; social, WebAuthn, passwordless; pairs with Hydra for OAuth2/OIDC; cloud option with easy self‑host migration | Build more UI/flows; multiple components to wire | Max UX control; Go‑native teams |
| **Keycloak** | OSS, Java; self‑host | Full **Admin REST API**; OIDC/SAML/LDAP; brokering; realms for multi‑tenancy; MFA; user federation | Heavy/complex ops; Java extensions; no official SaaS | Deep B2B SSO; many tenants |
| **FusionAuth** | OSS (free), Java; self‑host or managed | **API‑first** (admin UI built on REST APIs); OIDC/SAML/LDAP; social; MFA; **tenants**; webhooks; easier than Keycloak | Java footprint; some features enterprise‑gated | Balanced OSS IdP with easier ops |
| **Authentik** | OSS, Python; self‑host | OIDC/SAML/LDAP; policies/flows; MFA; API usable; quick Docker/K8s deploy | Smaller ecosystem; less proven at very large scale | Fast self‑hosted SSO for small–mid orgs |
| **SuperTokens** | OSS framework + SDKs; self‑host or cloud | API‑centric; drop‑in or custom UI; strong **session** security; social/passwordless | Not a full IdP/broker; enterprise SSO needs extra work | B2C & simple B2B with custom UX |
| **Auth0** | SaaS (Okta CIC) | Complete **Management & Auth APIs**; social + enterprise SSO; roles/MFA; fast time‑to‑value; strong compliance | Cost at scale; cloud lock‑in/residency; migration work | Rapid launch for B2C/B2B SSO |
| **Okta** | SaaS | Enterprise IAM; rich **REST APIs**; adaptive MFA; SCIM; governance | Higher cost; cloud‑only; more enterprise‑centric DX | Strict enterprise requirements |
| **AWS Cognito** | SaaS (AWS) | Fully API/SDK‑driven; user pools; social + **SAML** federation; MFA; scales; low cost | Dev UX/customization limits; AWS tie‑in; migration complexity | Cost‑efficient at scale on AWS |
| **Azure AD B2C** | SaaS (Azure) | API & Graph integration; OIDC/SAML; social; deep Microsoft ecosystem | Customization via policies has learning curve | Azure‑aligned enterprise scenarios |

---

## Architecture & Integration Blueprint (Headless)
- **Identity layer:** Use IdP **APIs** only (no hosted UI). Implement login/registration/MFA screens in PaaS; exchange via OIDC/SAML endpoints or direct login APIs.
- **Edge & services:** Traefik (optional ForwardAuth/OIDC for admin surfaces). Services verify **JWTs via JWKS**; **short‑lived access tokens** + refresh.
- **Authorization:** Hybrid **RBAC**. Org roles (`OrgAdmin/Dev/Viewer`) in IdP claims; per‑app roles (`Owner/Maintainer/Contributor/Reader`) in DB. Enforce with **Casbin**; optionally **OPA/Cerbos**. For large permission graphs, consider **Ory Keto** (Zanzibar‑style).
- **Enterprise SSO:** Per‑org **OIDC/SAML** connections; keep credentials on customer IdP. For strict on‑prem, offer dedicated self‑hosted IdP instance.
- **Individuals:** Social OIDC (Google, GitHub), account linking, email verification; **invitation** flows for collaboration.
- **Multi‑tenancy:** Realms/tenants (Keycloak/FusionAuth) or logical partitioning; clear data isolation and claim scoping.
- **Compliance/Audit:** Centralized authN/authZ **event logs**; access review reports; DPA/BAA; user deletion/export; regional hosting.

---

## Go Libraries & Components
- **OIDC/OAuth2:** `github.com/coreos/go-oidc`, `golang.org/x/oauth2`
- **SAML SP:** `github.com/crewjam/saml`
- **Sessions:** `gorilla/sessions`, `securecookie`
- **RBAC / Policy:** `github.com/casbin/casbin`, **OPA**, **Cerbos**
- **Gateway:** Traefik ForwardAuth; optional **Ory Oathkeeper**
- **Nomad:** Run IdP as Nomad jobs; distribute **JWKS**; optional OPA/Oathkeeper sidecars

---

## Recommended Paths
- **Managed‑first:** **Auth0** now → migrate to **Keycloak**/**FusionAuth** later; keep to OIDC standards; enforce with **Casbin**.
- **Self‑host‑first:** **FusionAuth** (balanced) or **Keycloak** (max features); tenants/realms; social; hybrid RBAC.
- **Headless‑by‑design:** **Ory Kratos (+ Hydra)** + **Cerbos/Casbin**; more build effort, **maximum UX control**.

---

## Security & Compliance Checklist
- **MFA** (TOTP/WebAuthn), password policies, rate limiting.
- **Key rotation (JWKS)**, HTTPS‑only, secure cookies, CSP.
- **Short tokens**, refresh rotation, session/device revocation.
- Centralized **audit logs**; SIEM export; GDPR erase/export.
