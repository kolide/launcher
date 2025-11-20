## Flare Upload Flow

The shipper supports uploading flares to both **GCS (Google Cloud Storage)** and **S3 (Amazon S3)** using presigned/signed URLs.

### Shipping initiated by user (cmd line):
```mermaid
sequenceDiagram
    launcher->>+server: presigned url request
    server->>+cloud: create presigned url (GCS or S3)
    cloud-->>+server: presigned url
    server-->>+launcher: presigned url
    launcher->>+launcher: generate artifact
    launcher->>+cloud: PUT artifact to presigned url
    cloud-->>+launcher: 200 OK or 204 No Content
```

### Shipping initiated by server:
```mermaid
sequenceDiagram
    server->>+cloud: create presigned url (GCS or S3)
    cloud-->>+server: presigned url
    server->>+launcher: do something and upload to (presigned url)
    launcher->>+launcher: generate artifact
    launcher->>+cloud: PUT artifact to presigned url
    cloud-->>+launcher: 200 OK or 204 No Content
```

## Storage Provider Support

- **GCS**: Returns HTTP 200 OK on successful upload
- **S3**: Returns HTTP 200 OK or 204 No Content on successful upload
