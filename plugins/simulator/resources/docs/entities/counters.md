# Counters

Counters in the Simulator.Company platform provide high-performance metrics tracking and aggregation across different time intervals.

## Overview

The Counters system uses ScyllaDB's counter columns to efficiently track and aggregate metrics in real-time. It supports various time-based aggregations (minutes, hours, days, years) and can track unique values for advanced analytics.

## Counter Types

### Simple Counters

Simple counters track cumulative values with the following tables:

| Table | Purpose | Time Granularity |
|-------|---------|------------------|
| counters | Yearly aggregation | Year |
| counters_minutes | Per-minute metrics | Minute |
| counters_hours | Hourly aggregation | Hour |
| counters_days | Daily aggregation | Day |

### Unique Counters

Unique counters track distinct values (like unique visitors) with the following tables:

| Table | Purpose | Time Granularity |
|-------|---------|------------------|
| counters_uniq | Track unique values by year | Year |
| counters_uniq_minutes | Track unique values by minute | Minute |
| counters_uniq_hours | Track unique values by hour | Hour |
| counters_uniq_last_value | Store the most recent unique value | N/A |

## Properties

### Simple Counters

| Property | Type | Description |
|----------|------|-------------|
| key | Text | Counter identifier (typically includes entity type and ID) |
| year/y_m/y_m_d_h | Text | Time period identifier (varies by granularity) |
| time | Timestamp/Date | Specific timestamp or date for the counter |
| amount | Counter | Cumulative amount value |
| hold | Counter | Amount on hold (for financial metrics) |
| count | Counter | Number of occurrences |

### Unique Counters

| Property | Type | Description |
|----------|------|-------------|
| key | Text | Counter identifier |
| year/y_m/y_m_d_h | Text | Time period identifier |
| time | Timestamp | Specific timestamp for the counter |
| uniq | Text/Set\<Text\> | Unique identifier or set of unique identifiers |

## API Endpoints

For detailed API documentation on counters, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Counters API Documentation](https://doc.simulator.company/#tag/counters)
[Metrics API Documentation](https://doc.simulator.company/#tag/metrics)

The API provides endpoints for:

- Getting counter values for specific keys
- Retrieving time series data for counter keys
- Incrementing counter values
- Tracking unique values for counter keys
- Aggregating counter data across time periods

All API requests require appropriate OAuth2 scopes (`control.events:metrics.readonly` for read operations and `control.events:metrics.management` for write operations).

## Use Cases

Counters are used throughout the platform for various purposes:

- **Financial Metrics**: Track account balances, transaction volumes, and holds
- **User Activity**: Monitor user engagement, session counts, and unique visitors
- **Performance Monitoring**: Track API calls, response times, and error rates
- **Business Analytics**: Measure conversion rates, funnel progression, and business KPIs

## Database Structure

The counter tables use ScyllaDB's counter column type for efficient atomic increments:

- Partitioned by key and time period for efficient time-based queries
- Clustering order by time for chronological retrieval
- Optimized for high write throughput and aggregation queries

## Example

### Counter Value

```json
{
  "key": "account:123456:transactions",
  "year": "2023",
  "amount": 15000,
  "hold": 500,
  "count": 42
}
```

### Time Series Data

```json
{
  "key": "account:123456:transactions",
  "timeseries": [
    {
      "time": "2023-05-01T00:00:00Z",
      "amount": 500,
      "count": 5
    },
    {
      "time": "2023-05-02T00:00:00Z",
      "amount": 750,
      "count": 8
    },
    {
      "time": "2023-05-03T00:00:00Z",
      "amount": 1200,
      "count": 12
    }
  ]
}
```

### Unique Values

```json
{
  "key": "workspace:789:active_users",
  "year": "2023",
  "unique_count": 156,
  "unique_values": ["user:123", "user:456", "user:789"]
}
```
