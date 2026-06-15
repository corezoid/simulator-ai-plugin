# Attachments

Attachments in the Simulator.Company platform provide file storage capabilities for actors, enabling users to associate documents, images, and other files with business entities.

## Overview

The Attachments system allows users to upload, store, and manage files associated with actors. It supports various file types, versioning, and access control to ensure secure and organized file management within the platform.

## Properties

### Attachment

| Property | Type | Description |
|----------|------|-------------|
| id | String | Unique identifier for the attachment |
| acc_id | String | Workspace ID the attachment belongs to |
| user_id | Integer | ID of the user who uploaded the attachment |
| filename | Text | Original filename of the attachment |
| size | Integer | File size in bytes |
| mime_type | Text | MIME type of the file |
| storage_type | Enum | Storage backend type (local, s3, etc.) |
| storage_path | Text | Path or identifier in the storage backend |
| hash | String | File hash for integrity verification |
| status | Enum | Attachment status (active, removed, etc.) |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |

### Actor to Attachments

The ActorToAttachments relationship tracks which attachments are associated with which actors:

| Property | Type | Description |
|----------|------|-------------|
| id | String | Unique identifier for the actor-attachment relationship |
| actor_id | String | ID of the actor |
| attach_id | String | ID of the attachment |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |

### Favorites Attachments

The FavoritesAttach relationship tracks which attachments are marked as favorites by users:

| Property | Type | Description |
|----------|------|-------------|
| id | String | Unique identifier for the favorite relationship |
| user_id | Integer | ID of the user |
| attach_id | String | ID of the attachment |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |

## Supported File Types

The platform supports various file types, including:

- **Documents** - PDF, Word, Excel, PowerPoint, etc.
- **Images** - JPEG, PNG, GIF, SVG, etc.
- **Archives** - ZIP, RAR, TAR, etc.
- **Text** - TXT, CSV, JSON, XML, etc.
- **Media** - Audio and video files

## API Endpoints

For detailed API documentation on attachments, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Attachments API Documentation](https://doc.simulator.company/#tag/attachments)
[Upload API Documentation](https://doc.simulator.company/#tag/upload)
[Download API Documentation](https://doc.simulator.company/#tag/download)

The API provides endpoints for:

- Uploading new attachments
- Retrieving attachment metadata
- Downloading attachment files
- Deleting attachments
- Getting all attachments for a specific actor
- Attaching files to actors
- Removing attachments from actors

All API requests require appropriate OAuth2 scopes (`control.events:attachments.readonly` for read operations and `control.events:attachments.management` for write operations).

## Reading attachment content (AI agent)

Listing endpoints (`getAttachments`, `getActorAttachments`) return only *metadata* —
including each file's storage `fileName`. To actually read a file's content, use the
**`readAttachment`** tool with that `fileName`. It downloads the bytes (through the
PAPI `/download/{fileName}` route, so access follows the caller's own permissions)
and adapts them to what the model can consume:

- **Textual** files (txt, md, csv, json, xml, yaml, source code, svg) → returned inline as text.
- **Images** (png, jpeg, gif, webp) → returned as a viewable image block.
- **PDFs and other binaries** → returned as an embedded resource (base64 blob) for the client to render.

Large files are capped: text is truncated past 256 KiB, images over 5 MiB are
returned as a metadata note instead of inline, and the overall download is bounded
at 24 MiB.

To read files attached to a **reaction / comment**, first list them with
`getActorAttachments` using the reaction's id, then call `readAttachment` for each
`fileName`. `readAttachment` is available in actor-scoped mode too, so a
reaction-triggered AI agent can read the files attached to the reaction it handles.

## Storage Backends

The platform supports multiple storage backends:

- **Local Storage** - Files stored on the local filesystem
- **Amazon S3** - Cloud storage using Amazon S3
- **Google Cloud Storage** - Cloud storage using Google Cloud
- **Azure Blob Storage** - Cloud storage using Microsoft Azure

## Database Structure

Attachments use multiple tables to implement their functionality:

- Attachment metadata is stored in the `attachments` table
- Actor-attachment relationships are stored in the `actor_to_attachments` table
- Favorite attachments are stored in the `favorites_attach` table

## Example

### Attachment Metadata

```json
{
  "id": "attachment_123456",
  "filename": "customer_contract.pdf",
  "size": 1024000,
  "mime_type": "application/pdf",
  "storage_type": "s3",
  "storage_path": "workspace_789/attachments/123456.pdf",
  "hash": "a1b2c3d4e5f6g7h8i9j0",
  "status": "active",
  "user_id": 42,
  "created_at": 1621459200,
  "updated_at": 1621459200
}
```

### Actor with Attachments

```json
{
  "id": "actor_789012",
  "title": "Customer Contract",
  "attachments": [
    {
      "id": "attachment_123456",
      "filename": "customer_contract.pdf",
      "size": 1024000,
      "mime_type": "application/pdf",
      "created_at": 1621459200
    },
    {
      "id": "attachment_234567",
      "filename": "contract_appendix.docx",
      "size": 512000,
      "mime_type": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
      "created_at": 1621545600
    }
  ]
}
```

## Security and Access Control

Attachments inherit access control from the actors they are associated with:

- Users must have access to the actor to access its attachments
- Workspace administrators can access all attachments in their workspace
- File access is logged for audit purposes
- Sensitive files can be encrypted at rest

## Usage in the Platform

Attachments are used throughout the platform for various purposes:

- **Document Management** - Storing and organizing business documents
- **Process Documentation** - Attaching supporting files to process steps
- **Media Storage** - Managing images and media files for actors
- **Data Import/Export** - Handling data files for import and export operations
- **Collaboration** - Sharing files between users working on the same processes

Attachments provide essential file management capabilities, enabling users to associate relevant documents and files with business entities in the platform.
