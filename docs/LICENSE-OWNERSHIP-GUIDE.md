# MySoc Updates Server — License Ownership Guide

| | |
|---|---|
| **Document Version** | 1.0.0 |
| **Applies To** | Updates Server 1.7.0+ |
| **Audience** | Platform admins, SOC operator staff |
| **Server** | `https://updates.mysoc.ai` |

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0.0 | 2026-08-19 | Updates Team | Initial guide for the operator/reseller/customer model (release 1.7.0) |

---

## 1. The Sales Flow, and How the Server Models It

The commercial reality the server mirrors:

1. **MySoc** sells the **mysoc platform** to a **SOC operator**.
2. The SOC operator sells to end **customers** — either **directly** or through a
   **reseller**. Resellers add customers in the mysoc platform; they deploy no
   software of their own.
3. Each customer is assigned one or more **siemcore** servers.
4. The customer can install **SWF** (SiemCore Windows Forwarder) on Windows
   servers to ship Sysmon and WMI logs into their siemcore.

```text
SOC operator  (platform license, runs one mysoc serving all its customers)
├── Customer A            — direct sale        (customer license)
│     └── siemcore-a1     — parent = operator's mysoc
│           ├── swf-01    — Sysmon/WMI forwarder
│           └── swf-02
└── Customer B            — sold via reseller "acme-channel"  (customer license)
      └── siemcore-b1
            └── swf-01
```

Two kinds of licenses express this:

| | Platform license | Customer license |
|---|---|---|
| Held by | The SOC operator | An end customer |
| Covers | The operator's mysoc node(s) | That customer's siemcore server(s) + swf forwarders |
| `type` | `mysoc-cloud` | `siemcore` or `siemcore-lite` |
| Key prefix | `MYSOC-…` | `SIEM-…` |
| `operator_id` | Its own `customer_id` | The operator it lives under |
| `reseller_id` / `reseller_name` | — | Optional; empty means a direct sale |

Every node covered by one license presents that license's key in
`X-License-Key`. **The key identifies the fleet; it never authorizes
administration or release uploads** (those need an admin/API key — see the
Admin Guide).

---

## 2. Creating Licenses (Dashboard)

Only users with the **admin** role can create or edit licenses
(**Dashboard → Licenses → Create License**).

### 2.1 Create the operator's platform license (once per operator)

1. Choose scope **SOC Operator (mysoc platform)**.
2. Enter the **Operator ID** (e.g. `cyfox-soc`) and **Operator Name**.
3. Products default to `mysoc-api`, `mysoc-frontend`.
4. Set the expiry and create. The key is issued as `MYSOC-…`, and
   `operator_id` is automatically set to the operator's own ID.

### 2.2 Create a customer license (one per customer)

1. Choose scope **Customer (siemcore + swf)**.
2. Enter the **Customer ID** and **Customer Name**.
3. Pick the **SOC Operator** from the dropdown (fed by existing platform
   licenses). "Unassigned (set later)" is allowed but the customer will sit in
   the *Unassigned* bucket of the tree until you fix it.
4. If the sale went through a channel, fill **Reseller ID** and **Reseller
   Name**; leave empty for a direct sale.
5. Products default to `siemcore-api`, `siemcore-collector`,
   `siemcore-frontend`, `swf`.
6. Set the expiry and create. The key is issued as `SIEM-…`. Hand this **one
   key** to the customer: their siemcore and every swf agent all use it.

### 2.3 Assign ownership on existing (legacy) licenses

Licenses created before 1.7.0 have no operator. On the license detail page,
the **Ownership** card shows *SOC Operator: Unassigned* — click **Edit**
(admin only), set the Operator ID (and reseller, if applicable), and save. The
tree regroups immediately.

---

## 3. Creating Licenses (API)

`POST /api/v1/admin/licenses` (admin JWT). Ownership fields are optional but
recommended:

```json
{
  "customer_id": "acme",
  "customer_name": "ACME Corp",
  "type": "siemcore",
  "operator_id": "cyfox-soc",
  "reseller_id": "chan-1",
  "reseller_name": "Acme Channel Ltd",
  "products": ["siemcore-api", "siemcore-collector", "siemcore-frontend", "swf"],
  "expires_at": "2027-08-19T00:00:00Z"
}
```

For a platform license use `"type": "mysoc-cloud"`; `operator_id` then
defaults to the license's own `customer_id`.

Update ownership later with a partial `PUT /api/v1/admin/licenses/{id}`:

```json
{ "operator_id": "cyfox-soc", "reseller_id": "chan-1", "reseller_name": "Acme Channel Ltd" }
```

---

## 4. Wiring the Agents

Nothing changed in the agent protocol in 1.7.0. Each agent self-reports its
tier and parent on every heartbeat and update check:

| Node | `product_tier` | `parent_id` (config) → `parent_instance_id` (wire) | `license_key` |
|------|----------------|-----------------------------------------------------|----------------|
| mysoc | `mysoc` | *(omit — root)* | operator's platform license |
| siemcore | `siemcore` | **the operator's mysoc `instance_id`** | the customer's license |
| swf | `swf` | that customer's siemcore `instance_id` | the **same** customer license |

Note that a siemcore's parent legitimately lives under a *different* license
(the operator's). The server resolves parent links across the whole fleet, so
this is a normal link, not an error.

---

## 5. Reading the Tree

**Dashboard → Instances → Tree view**, or `GET /api/v1/instances/tree` (JWT).
The fleet is grouped as **operator → customer → siemcore → swf**:

- **Platform (mysoc)** — the operator's own nodes, listed at the operator level.
- **Customer cards** — one per customer license, showing the masked key, node
  count, and a reseller badge when the sale went through a channel. A customer
  appears as soon as its license exists, even before any instance enrolls.
- **`orphan` badge** — the node declared a parent that is **not enrolled
  anywhere** in the fleet. Typical causes: the parent has not enrolled yet, or
  the `parent_id` in the agent config has a typo.
- **Unassigned** (last group) — licenses without an `operator_id`, plus an
  *Unlicensed / unbound* bucket for instances that presented no (valid)
  license key. Both indicate cleanup work, not errors.

---

## 6. FAQ

**Does the SWF team need to change anything for 1.7.0?**
No. Config and protocol are unchanged. The only production-wiring rule (from
1.6.0) still applies: swf's `parent_id` is the customer's siemcore, and the
siemcore's `parent_id` is the operator's mysoc instance id.

**Can one customer have several siemcore servers?**
Yes. They all share the customer's license key and each declares the
operator's mysoc as parent.

**Can a license upload releases?**
No. `X-License-Key` never authorizes `POST /releases`; uploads require an
admin/API key (see the Admin Guide and API Contract §5.9).

**What happens if I never assign an operator?**
Everything keeps working — validation, updates, heartbeats. The license and
its instances simply render in the *Unassigned* group of the tree.
