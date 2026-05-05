# Search

Search in the Simulator.Company platform provides powerful full-text search capabilities for actors and other entities.

## Overview

The Search system enables users to find actors and other entities quickly using full-text search. It maintains a separate optimized index of searchable content with access control information to ensure users only see results they have permission to access.

## ActorsSearch Model

The ActorsSearch model stores indexed search data for actors:

| Property | Type | Description |
|----------|------|-------------|
| id | String | Unique identifier matching the actor ID |
| ref | String | External reference identifier |
| acc_id | String | Workspace ID the actor belongs to |
| form_id | Integer | ID of the form template used by the actor |
| search | TSVECTOR | PostgreSQL text search vector for efficient full-text search |
| access | JSON | Access control information determining who can see this actor |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |

## Search Functionality

The platform's search system provides several key features:

- **Full-Text Search**: Find actors by title, description, and other text fields
- **Access-Controlled Results**: Only return results the user has permission to see
- **Form-Specific Filtering**: Filter results by form type
- **Relevance Ranking**: Order results by relevance to the search query
- **Prefix Matching**: Find results that start with the search term

## API Endpoints

### Search Actors

```
GET /papi/1.0/search/{accId}
```

Scopes: `control.events:actors.readonly`

Searches for actors matching the query within a workspace.

### Search Actors by Form

```
GET /papi/1.0/search/{accId}/form/{formId}
```

Scopes: `control.events:actors.readonly`

Searches for actors of a specific form type matching the query.

### Global Search

```
GET /papi/1.0/search/global/{accId}
```

Scopes: `control.events:actors.readonly`

Searches across all searchable entities (actors, forms, etc.) matching the query.

## Database Structure

The search system uses PostgreSQL's full-text search capabilities:

- TSVECTOR type for efficient text indexing and searching
- GIN index for fast full-text search operations
- Automatic indexing via database triggers when actors are created or updated
- Weighted search with title (A) having higher priority than description (D)

## Implementation Details

The search system is implemented using:

- PostgreSQL's `to_tsvector` and `to_tsquery` functions
- Database triggers that automatically update search vectors when actors change
- Access control filtering using JSONB operators
- Relevance ranking using PostgreSQL's `ts_rank` function

## Example

### Search Query

```
GET /papi/1.0/search/workspace_123?q=customer+invoice
```

### Search Results

```json
{
  "results": [
    {
      "id": "actor_456789",
      "title": "Customer Invoice #12345",
      "description": "Monthly subscription invoice",
      "form_id": 42,
      "form_title": "Invoice",
      "relevance": 0.89,
      "created_at": 1621459200,
      "updated_at": 1621545600
    },
    {
      "id": "actor_123456",
      "title": "New Customer Onboarding",
      "description": "Process for invoice generation",
      "form_id": 17,
      "form_title": "Process",
      "relevance": 0.65,
      "created_at": 1621372800,
      "updated_at": 1621459200
    }
  ],
  "total": 2,
  "page": 1,
  "per_page": 20
}
```

## Search Optimization

The search system is optimized for performance:

- Separate search table to avoid impacting transactional performance
- Asynchronous indexing for large text fields
- Caching of frequent search queries
- Pagination of search results to limit resource usage
