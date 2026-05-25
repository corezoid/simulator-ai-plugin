# Forms

Forms in the Simulator.Company platform define the structure and behavior of actors, providing reusable templates for data collection and validation.

## Overview

Forms (also known as Smart Forms or CDU) serve as templates that define the structure, fields, and validation rules for actors. They enable consistent data collection and provide a foundation for creating structured business processes.

## Properties

| Property | Type | Description |
|----------|------|-------------|
| id | Integer | Unique identifier for the form |
| acc_id | String | Workspace ID the form belongs to |
| user_id | Integer | ID of the user who created the form |
| title | Text | Display title of the form |
| description | Text | Detailed description of the form's purpose |
| sections | JSON | Form sections containing fields and their properties |
| color | String | Color associated with the form (hex code) |
| picture | Text | URL or path to the form's image |
| tags | JSON Array | Tags for categorization |
| settings | JSON | Form-specific settings |
| parent_id | Integer | ID of the parent form (for form inheritance) |
| status | Enum | Form status (active, removed, etc.) |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |

## Form Sections and Fields

Forms are organized into sections, each containing fields with specific properties:

### Section Structure

| Property | Type | Description |
|----------|------|-------------|
| title | String | Section title |
| content | Array | Array of field definitions |

### Field Types

Forms support various field types:

- **Text** - Single-line or multi-line text input
- **Number** - Integer or decimal number input
- **Select** - Single-selection dropdown
- **MultiSelect** - Multiple-selection dropdown
- **Checkbox** - Boolean true/false input
- **Radio** - Single selection from multiple options
- **Date** - Date picker
- **Time** - Time picker
- **File** - File upload
- **Table** - Tabular data entry
- **Reference** - Reference to another actor
- **Formula** - Calculated value based on other fields

### Field Properties

Each field has properties that define its behavior:

| Property | Type | Description |
|----------|------|-------------|
| id | String | Field identifier |
| class | String | Field type (edit, select, checkbox, etc.) |
| title | String | Display label for the field |
| type | String | Data type (text, int, float, etc.) |
| value | Mixed | Default value |
| options | Array | Available options for select fields |
| required | Boolean | Whether the field is required |
| visibility | String | Field visibility (visible, hidden, disabled) |
| regexp | String | Regular expression for validation |
| errorMsg | String | Custom error message for validation failures |
| extra | Object | Additional field-specific properties |

## API Endpoints

For detailed API documentation on forms, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Forms API Documentation](https://doc.simulator.company/#tag/forms)

The API provides endpoints for:

- Getting all forms in a workspace
- Retrieving specific form details
- Creating new forms
- Updating existing forms
- Deleting forms
- Managing form inheritance and relationships

All API requests require appropriate OAuth2 scopes (`control.events:forms.readonly` for read operations and `control.events:forms.management` for write operations).

### Clone Form

```
POST /papi/1.0/forms/{formId}/clone
```

Scopes: `control.events:forms.management`

Creates a copy of an existing form.

## Form Inheritance

Forms support inheritance through the `parent_id` property:

- Child forms inherit sections and fields from their parent
- Child forms can override inherited fields
- Changes to parent forms can be propagated to child forms
- Multiple levels of inheritance are supported

## Form Accounts

Forms can define default account structures for actors:

- Account names and types
- Default currencies
- Initial balances
- Formula-based calculations

## Database Structure

Forms are stored in the `forms` table with the following structure:

- Primary key on `id`
- Foreign key relationships to workspaces and users
- Indexed for efficient querying by acc_id and parent_id
- JSON storage for sections, fields, and settings

## Example

### Basic Form

```json
{
  "id": 42,
  "title": "Customer",
  "description": "Customer information form",
  "color": "#3498db",
  "sections": [
    {
      "title": "General Information",
      "content": [
        {
          "id": "name",
          "class": "edit",
          "title": "Customer Name",
          "required": true,
          "visibility": "visible"
        },
        {
          "id": "email",
          "class": "edit",
          "title": "Email Address",
          "regexp": "^[\\w-\\.]+@([\\w-]+\\.)+[\\w-]{2,4}$",
          "errorMsg": "Please enter a valid email address",
          "required": true,
          "visibility": "visible"
        }
      ]
    },
    {
      "title": "Additional Information",
      "content": [
        {
          "id": "type",
          "class": "select",
          "title": "Customer Type",
          "value": "",
          "options": [
            {"title": "Individual", "value": "individual"},
            {"title": "Business", "value": "business"}
          ],
          "required": true,
          "visibility": "visible"
        },
        {
          "id": "notes",
          "class": "edit",
          "type": "text",
          "extra": {"multiline": true, "rows": 5},
          "title": "Notes",
          "required": false,
          "visibility": "visible"
        }
      ]
    }
  ],
  "settings": {},
  "created_at": 1621459200,
  "updated_at": 1621545600
}
```

## Usage in the Platform

Forms are used throughout the platform for various purposes:

- **Data Structure Definition** - Defining the structure and validation rules for actors
- **User Interface Generation** - Automatically generating UI components based on form definitions
- **Validation** - Enforcing data integrity through field validation rules
- **Default Values** - Providing initial values for actor fields
- **Conditional Logic** - Implementing dynamic behavior based on field values

Forms (Smart Forms/CDU) are a central component of the platform, enabling structured data collection and consistent business processes.
