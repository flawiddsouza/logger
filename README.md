### Dev (Run)
```bash
go run main.go
```

### Prod (Build)
```bash
go build -o dist/logger
```

### Prod (Run)
```bash
./dist/logger
```


### Usage
Store Log:
```
POST http://localhost:4964/log
{
    "group": "a",
    "stream": "b",
    "timestamp": "c",
    "message": "d"
}
```

Retrieve Log:
```
GET http://localhost:4964/log?group=a&stream=b
[
	{
		"timestamp": "c",
		"message": "d"
	},
	{
		"timestamp": "c",
		"message": "d"
	},
	{
		"timestamp": "c",
		"message": "d"
	}
]
```
