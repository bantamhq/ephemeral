# Ephemeral API Reference

## Authentication

All routes except public ones require a Bearer token in the `Authorization` header:

```
Authorization: Bearer eph_...
```

---

## Public (No Auth)

| Method | Route | Parameters |
|--------|-------|------------|
| `GET` | `/health` | - |
| `GET` | `/api/v1/auth/config` | - |

---

## Admin Routes (Admin Token Required)

### Namespaces

| Method | Route | Parameters |
|--------|-------|------------|
| `GET` | `/api/v1/admin/namespaces` | `?cursor=`, `?limit=` |
| `POST` | `/api/v1/admin/namespaces` | Body: `{name, repo_limit?, storage_limit_bytes?, external_id?}` |
| `GET` | `/api/v1/admin/namespaces/{name}` | - |
| `DELETE` | `/api/v1/admin/namespaces/{name}` | - |

### Tokens

| Method | Route | Parameters |
|--------|-------|------------|
| `GET` | `/api/v1/admin/tokens` | `?cursor=`, `?limit=` |
| `GET` | `/api/v1/admin/tokens/{id}` | - |
| `DELETE` | `/api/v1/admin/tokens/{id}` | - |

### Users

| Method | Route | Parameters |
|--------|-------|------------|
| `GET` | `/api/v1/admin/users` | `?cursor=`, `?limit=` |
| `POST` | `/api/v1/admin/users` | Body: `{namespace_id}` (required) |
| `GET` | `/api/v1/admin/users/{id}` | - |
| `DELETE` | `/api/v1/admin/users/{id}` | - |

### User Tokens

| Method | Route | Parameters |
|--------|-------|------------|
| `GET` | `/api/v1/admin/users/{id}/tokens` | - |
| `POST` | `/api/v1/admin/users/{id}/tokens` | Body: `{expires_in_seconds?}` |

### User Namespace Grants

| Method | Route | Parameters |
|--------|-------|------------|
| `POST` | `/api/v1/admin/users/{id}/namespace-grants` | Body: `{namespace_id, allow[], deny[]?}` |
| `GET` | `/api/v1/admin/users/{id}/namespace-grants` | - |
| `GET` | `/api/v1/admin/users/{id}/namespace-grants/{nsID}` | - |
| `DELETE` | `/api/v1/admin/users/{id}/namespace-grants/{nsID}` | - |

### User Repo Grants

| Method | Route | Parameters |
|--------|-------|------------|
| `POST` | `/api/v1/admin/users/{id}/repo-grants` | Body: `{repo_id, allow[], deny[]?}` |
| `GET` | `/api/v1/admin/users/{id}/repo-grants` | - |
| `GET` | `/api/v1/admin/users/{id}/repo-grants/{repoID}` | - |
| `DELETE` | `/api/v1/admin/users/{id}/repo-grants/{repoID}` | - |

---

## User Routes (User Token Required)

### Namespaces

| Method | Route | Parameters |
|--------|-------|------------|
| `GET` | `/api/v1/namespaces` | Lists namespaces user has grants for |
| `PATCH` | `/api/v1/namespaces/{name}` | Body: `{name?, repo_limit?, storage_limit_bytes?}` (requires `namespace:admin`) |
| `DELETE` | `/api/v1/namespaces/{name}` | Requires `namespace:admin` |
| `GET` | `/api/v1/namespaces/{name}/grants` | Requires `namespace:admin` |

### Repos

| Method | Route | Parameters |
|--------|-------|------------|
| `GET` | `/api/v1/repos` | `?namespace=` (optional, defaults to all accessible), `?cursor=`, `?limit=`, `?expand=folders` |
| `POST` | `/api/v1/repos` | Body: `{name, description?, public, namespace?}` (namespace defaults to user's primary) |
| `GET` | `/api/v1/repos/{id}` | - |
| `PATCH` | `/api/v1/repos/{id}` | Body: `{name?, description?, public?}` |
| `DELETE` | `/api/v1/repos/{id}` | Requires `repo:admin` |

### Repo Refs

| Method | Route | Parameters |
|--------|-------|------------|
| `POST` | `/api/v1/repos/{id}/refs` | Body: `{name, sha, type}` |
| `PATCH` | `/api/v1/repos/{id}/refs/{refType}/*` | Body: `{sha}` |
| `DELETE` | `/api/v1/repos/{id}/refs/{refType}/*` | - |
| `PUT` | `/api/v1/repos/{id}/default-branch` | Body: `{branch}` |

### Repo Folders

| Method | Route | Parameters |
|--------|-------|------------|
| `GET` | `/api/v1/repos/{id}/folders` | - |
| `POST` | `/api/v1/repos/{id}/folders` | Body: `{folder_ids[]}` |
| `PUT` | `/api/v1/repos/{id}/folders` | Body: `{folder_ids[]}` (replaces all) |
| `DELETE` | `/api/v1/repos/{id}/folders/{folderID}` | - |

### Folders

| Method | Route | Parameters |
|--------|-------|------------|
| `GET` | `/api/v1/folders` | `?namespace=` (optional, defaults to user's primary), `?cursor=`, `?limit=` |
| `POST` | `/api/v1/folders` | Body: `{name, color?, namespace?}` (namespace defaults to user's primary) |
| `GET` | `/api/v1/folders/{id}` | - |
| `PATCH` | `/api/v1/folders/{id}` | Body: `{name?, color?}` |
| `DELETE` | `/api/v1/folders/{id}` | `?force=true` to delete non-empty |

---

## Content API (Optional Auth, Public Repos Accessible)

| Method | Route | Parameters |
|--------|-------|------------|
| `GET` | `/api/v1/repos/{id}/readme` | - |
| `GET` | `/api/v1/repos/{id}/refs` | - |
| `GET` | `/api/v1/repos/{id}/commits` | `?ref=`, `?cursor=`, `?limit=` |
| `GET` | `/api/v1/repos/{id}/commits/{sha}` | - |
| `GET` | `/api/v1/repos/{id}/commits/{sha}/diff` | - |
| `GET` | `/api/v1/repos/{id}/compare/{base}...{head}` | - |
| `GET` | `/api/v1/repos/{id}/tree/{ref}/*` | - |
| `GET` | `/api/v1/repos/{id}/blob/{ref}/*` | - |
| `GET` | `/api/v1/repos/{id}/blame/{ref}/*` | - |
| `GET` | `/api/v1/repos/{id}/archive/{ref}` | - |

---

## Git Protocol

| Route | Description |
|-------|-------------|
| `/git/{namespace}/{repo}.git/*` | Git HTTP protocol (clone, push, fetch) |
| `/git/{namespace}/{repo}.git/info/lfs/*` | Git LFS API (if enabled) |

---

## Permissions

Permission strings used in grants:

| Permission | Description |
|------------|-------------|
| `repo:read` | Read repository contents |
| `repo:write` | Push to repository |
| `repo:admin` | Delete repo, manage settings (implies read/write) |
| `namespace:read` | List repos/folders in namespace |
| `namespace:write` | Create repos/folders (implies read) |
| `namespace:admin` | Delete namespace, manage grants (implies read/write) |
