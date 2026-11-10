# Moving the data of the current systems

The importers are **idempotent** (a `legacy_map` table remembers old ids, so re-running never
duplicates), run in **one transaction** (all or nothing) and support `-dry-run`.
Rehearse on a copy first. The old systems are never modified.

## 0. Prepare

1. Deploy the new system and sign in (`make up`).
2. Put copies of the old data in `./legacy/`:
   * Warehouse: `warehouse.db` (the `data/` folder of WareHouseManager) and the photo folder `public/img/parts` → `./legacy/parts`
   * Attendance: `attendance.db` (SQLite) — or use a Postgres URL if it already runs on Postgres
   * Guarantee: the Postgres database. Either `pg_dump` it into a scratch database, or make it reachable
     from the server and pass its URL (`host.docker.internal` reaches Postgres on the same host).
3. `make backup` (the import script also takes a safety backup).

## 1. Order (recommended)

```bash
make import-warehouse  ARGS="-src sqlite:/legacy/warehouse.db -images /legacy/parts"      # parts, stock, documents, users
make import-guarantee  ARGS="-src postgres://user:pass@host.docker.internal:5432/guarantee_db"
make import-attendance ARGS="-src sqlite:/legacy/attendance.db"
```

Do the warehouse first: the guarantee system's "repair components" are then **matched to warehouse
parts by name** (Arabic/Persian letter variants normalised) instead of becoming duplicates.
The command prints how many matched; unmatched ones are created as parts named like the old component
(`RC-<id>`), ready to merge or rename in *Catalog → Parts*.

Add `-dry-run` to any of them to see the counts without saving.

## 2. What is carried over

| Old system | Imported | Notes |
|---|---|---|
| **Warehouse** | users, items → parts, stock levels, workers, every document and line, doc numbers | **QR tokens kept** → printed labels still scan. Part photos copied to the uploads volume. Stock = the old item stock (opening balance); numbering continues after the highest old document number. |
| **Guarantee** | admins/technical users, technicians (with status), categories, products (+code rules), repair catalog, customers, guarantees (incl. golden periods), repairs + parts + services, part requests, shipments + invoice/payment state, polls → surveys (questions, recipients, answers), SMS patterns | Products arrive **unpublished** in the shop and as *price on request* until you review them. Technician balances are rebuilt in the new ledger from invoiced/paid shipments. Imported guarantee codes are never re-validated, so old seasonal codes stay valid. |
| **Attendance** | users, departments, shifts, holidays, employees (+PIN hash), face vectors, devices, **all punches**, leaves, tasks and logs, fingerprint templates/slots | Face vectors are copied unchanged (same vector space as the old kiosk). |

Not imported (by design): old audit trails, app settings, sessions. Keep the old databases archived.

## 3. Passwords, PINs and device keys

* **User passwords**: bcrypt hashes (guarantee) are used directly; scrypt (warehouse) and PBKDF2
  (attendance) are stored as `legacy_hash`, verified at the user's first login and **replaced by bcrypt
  automatically**. Nobody has to reset anything. Imported warehouse users are flagged "must change password" as before.
* **Employee PINs** (attendance, PBKDF2) keep working as imported.
* **Kiosk tablets / fingerprint readers** authenticate with a key hashed with the old system's
  `SECRET_KEY`. Set `LEGACY_DEVICE_KEY_SECRET=<old SECRET_KEY>` in `.env` and `make up`; existing devices
  keep working with no re-registration. Point them at the new server address (`/api/v1/kiosk/*`).
  Rotate keys later from *HR → Devices*.
* Usernames clash? The importer appends `_2`, `_3`… (e.g. two systems both had `admin`).

## 4. Cut-over checklist

1. Freeze the old systems (read-only) at an agreed time; take final backups.
2. Run the three imports for real; run `make stats` and compare with the old counts.
3. Spot-check: a guarantee code in *public lookup*, a technician balance, a stock count of 3 parts, last week's attendance.
4. Review imported products (publish, set prices), SMS patterns (Settings → SMS: the `body_id`s are imported), roles of imported users.
5. Print nothing — old QR labels still work. Switch kiosk tablets to the new address.
6. Keep the old systems archived for a few months.

## 5. Things to decide with the business

* **Technician stock starts empty.** The old system never tracked what a technician held. After cut-over,
  deliver a starting stock through part requests, or create an "opening" transfer.
* **Old repairs' parts are imported as used from "stock"** with their return state derived from
  shipments; parts not yet sent and belonging to approved/submitted repairs show up in the technician's "send" list.
* The guarantee system's `Technical` role became `technical` (no delete, no user management).
