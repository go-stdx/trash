# trash

Go package to trash files, based on this [specification](https://specifications.freedesktop.org/trash-spec/1.0/). 

```
package main

import (
	"github.com/go-stdx/trash"
)

func main() {
	_, err := trash.Put("/home/rok/src/github.com/go-stdx/trash/xxx")
	if err != nil {
		panic(err)
	}
}
```
