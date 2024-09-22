# go2type

A Go to TypeScript API client generator.

## Overview

go2type is a tool that generates TypeScript API clients from Go server code. It parses Go structs and handler functions, generating corresponding TypeScript types and API client functions.

The generator can produce standard query functions, React hooks, or React Query hooks based on configuration.

## Features

- Generates TypeScript types from Go structs
- Creates API client functions from Go handler functions
- Supports generation of:
  - Standard query functions
  - React hooks
  - React Query hooks
- Customizable type mappings
- Automatically parse time.Time as Date objects
- Prettier formatting support
- Automatic configuration initialization

## Installation

To install go2type, use the following command:

```
go install github.com/dx314/go2type@latest
```

## Usage

go2type provides two main commands:

1. `init`: Initialize a new configuration file
2. `generate`: Generate TypeScript files based on the configuration

### Initializing Configuration

To create a new configuration file, run:

```
go2type init
```

This command will:
- Check for existing Go handlers in your project
- Detect the presence of React or React Query
- Find Prettier in your project or system PATH
- Create a `go2type.yaml` file with default settings

### Generating TypeScript Files

To generate TypeScript files based on your configuration, run:

```
go2type generate
```

This command will:
- Load the configuration from `go2type.yaml`
- Parse the specified Go packages
- Generate TypeScript types and API client functions
- Format the output using Prettier (if available)

### Configuration

The `go2type.yaml` file contains the following fields:

```yaml
# Authentication token for API requests
auth_token: "myAuthToken"

# Path to Prettier executable for code formatting
prettier_path: "path/to/prettier"

# Hook generation mode: "false" (standard), "true" (React hooks), or "react-query"
hooks: "false"

# Use Date objects for time.Time fields, defaults to false
# Adds a date parsing function to the generated API client
use_date_object: true

# List of packages to process
packages:
  - path: "./internal/api"
    output_path: "./frontend/src/api/generated.ts"
    type_mappings:
      "custom.Type": "CustomType"
```

## Go Code Examples

Handler function with comments:

```go
// @Method GET
// @Path /users/:id
// @Input GetUserInput
// @Output User
// @Header input:X-Custom-Header
// @Header localStorage:X-Auth-Token
// @Header sessionStorage:X-Session-ID
func GetUserHandler(w http.ResponseWriter, r *http.Request) {
    // Handler implementation
}
```

In the example above, the `@Header` directive specifies the source of each header:
- `input:X-Custom-Header`: The header value will be provided as an input parameter
- `localStorage:X-Auth-Token`: The header value will be retrieved from localStorage
- `sessionStorage:X-Session-ID`: The header value will be retrieved from sessionStorage

If no source is specified, it defaults to `input`.

Go struct:

```go
type User struct {
    ID        int       `json:"id"`
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    CreatedAt time.Time `json:"created_at"`
}
```

## Generated TypeScript

Types:

```typescript
export type User = {
  id: number;
  name: string;
  email: string;
  created_at: Date;
};
```

Standard query function:

```typescript
export const GetUserQuery = async (
  id: string,
  input: GetUserInput,
  X_Custom_Header: string
): Promise<User> => {
  const headers = {
    'X-Custom-Header': X_Custom_Header,
    'X-Auth-Token': localStorage.getItem('X-Auth-Token') || '',
    'X-Session-ID': sessionStorage.getItem('X-Session-ID') || '',
  };
  // Implementation
};
```

React hook:

```typescript
export const useGetUser = (
  id: string,
  input: GetUserInput,
  X_Custom_Header: string
) => {
  // Implementation using useState and useEffect
};
```

React Query hook:

```typescript
export const useGetUser = (
  id: string,
  input: GetUserInput,
  X_Custom_Header: string,
  options?: Omit<UseQueryOptions<User, APIError>, "queryKey" | "queryFn">
) =>
  useQuery<User, APIError>({
    queryKey: ["GetUser", id, input, X_Custom_Header],
    queryFn: () => GetUserQuery(id, input, X_Custom_Header),
    ...options,
  });
```


## License

MIT License

Copyright (c) 2024 Alex Dunmow

## Contributing

Contributions are welcome! Please submit a pull request or open an issue to discuss potential changes.
