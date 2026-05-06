# System Data

System Data in the Simulator.Company platform encompasses various supporting data structures that enable core platform functionality such as notifications, task scheduling, and user activity tracking.

## Overview

The System Data components use ScyllaDB for high-performance storage of operational data that supports the platform's functionality. These components include task callbacks for scheduled operations, push notification tokens, recent object tracking, and coordinate transactions.

## Components

### Task Callbacks

Task Callbacks manage scheduled operations and webhook notifications:

| Property | Type | Description |
|----------|------|-------------|
| shard | SmallInt | Shard identifier for distributed processing |
| date | Date | Scheduled execution date |
| execute_time | TimeUUID | Precise execution time with uniqueness |
| created_at | Timestamp | When the task was created |
| ref | Text | Reference identifier for the task |
| callback_type | Text | Type of callback (webhook, email, etc.) |
| callback_url | Text | URL to call when task executes |
| data | Text | Payload data for the callback |
| attempt | SmallInt | Number of execution attempts |
| user_id | Integer | ID of the user who created the task |

### Push Tokens

Push Tokens store device information for sending push notifications:

| Property | Type | Description |
|----------|------|-------------|
| user_id | Integer | ID of the user |
| device_id | Text | Unique device identifier |
| push_token | Text | Token for sending push notifications |
| platform | Text | Device platform (iOS, Android, etc.) |
| push_token_voip | Text | Token for VoIP push notifications |
| updated_at | Timestamp | When the token was last updated |

### Recent Objects

Recent Objects track user interactions with objects for activity feeds and recent items lists:

| Property | Type | Description |
|----------|------|-------------|
| acc_id | Text | Workspace ID |
| user_id | Integer | ID of the user |
| obj_type | Text | Type of object (actor, form, etc.) |
| obj_id | Text | ID of the object |
| form_id | Integer | ID of the associated form |
| time | Timestamp | When the interaction occurred |

### Actor Coordinates Transactions

Actor Coordinates Transactions track changes to actor positions in graph visualizations:

| Property | Type | Description |
|----------|------|-------------|
| id | Integer | Actor ID |
| x | Double | X-coordinate position |
| y | Double | Y-coordinate position |
| diff_x | Double | Change in X-coordinate |
| diff_y | Double | Change in Y-coordinate |
| user_id | Integer | ID of the user who made the change |
| time | Timestamp | When the change occurred |

## API Endpoints

### Task Callbacks

```
POST /papi/1.0/tasks/schedule
```

Scopes: `control.events:system.management`

Schedules a new task for execution.

```
GET /papi/1.0/tasks/{taskRef}
```

Scopes: `control.events:system.readonly`

Retrieves information about a scheduled task.

### Push Notifications

```
POST /papi/1.0/notifications/register-device
```

Scopes: `control.events:notifications.management`

Registers a device for push notifications.

```
POST /papi/1.0/notifications/send
```

Scopes: `control.events:notifications.management`

Sends a push notification to users.

### Recent Objects

```
GET /papi/1.0/recent
```

Scopes: `control.events:actors.readonly`

Retrieves recently accessed objects for the current user.

## Database Structure

The system data tables use ScyllaDB for high-performance, distributed storage:

- Task Callbacks are partitioned by shard and date for distributed processing
- Push Tokens are indexed by user_id and device_id for quick lookups
- Recent Objects use a composite partition key for efficient filtering
- Actor Coordinates Transactions are ordered by time for chronological retrieval

## Examples

### Task Callback

```json
{
  "shard": 1,
  "date": "2023-05-30",
  "execute_time": "2023-05-30T15:30:00.000Z",
  "created_at": "2023-05-30T10:15:00.000Z",
  "ref": "webhook_123456",
  "callback_type": "webhook",
  "callback_url": "https://example.com/webhook",
  "data": "{\"event\":\"transaction_completed\",\"transaction_id\":\"tx_789\"}",
  "attempt": 0,
  "user_id": 42
}
```

### Push Token

```json
{
  "user_id": 42,
  "device_id": "device_123456",
  "push_token": "fcm-token-abcdef123456",
  "platform": "android",
  "updated_at": "2023-05-30T10:15:00.000Z"
}
```

### Recent Object

```json
{
  "acc_id": "workspace_789",
  "user_id": 42,
  "obj_type": "actor",
  "obj_id": "actor_123456",
  "form_id": 5,
  "time": "2023-05-30T10:15:00.000Z"
}
```

## System Integration

The System Data components integrate with other platform systems:

- **Task Callbacks** enable scheduled operations and external integrations
- **Push Tokens** support real-time notifications to mobile and web clients
- **Recent Objects** power activity feeds and user experience features
- **Actor Coordinates** support collaborative graph editing and visualization

These components are essential for the platform's real-time collaboration features, external integrations, and user experience optimizations.
