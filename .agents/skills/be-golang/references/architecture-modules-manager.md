# Modules Manager

## Overview

Modules Manager contains all main logic for all modules of the service. It will receive request from Controller, call Adapter to access data from external systems, process data and return to Controller.

## File Structure
```
internal/modules/
  <module-name>/
    <module-name>_manager.go
  helper/
    <purpose>_helper.go
  modules_manager.go
```

- Each module have it own folder in modules-manager folder.
- Each module have it own manager file, which will handle all request for that module. But keep the modules size small (< 500 lines), if module is too big, split it into smaller modules.
- For common function that need to be used in multiple modules, create a helper file in helper folder.

## Helper

Helper is function that used by multiple modules in modules-manager. If one logic repeat in multiple modules, extract it to helper.

### Rules
- Manager Helper function should be small (<100 lines) and focused on one task. It should not contain any business logic.
- Multiple Manager Helper function can be group into single file if they are related to each other (Like `id_generator_helper.go`, `time_parser_helper.go`, etc).

## Modules

A module focuses on implementing business logic for a single domain. It can access the Adapter to get data from external systems. It can access another module's interface to use its functionality.

- Each modules should only inject necessary adapter and other module, keep it as small as possible.
- Module manager should only extract the interface, not the implementation.
- All module manager should be init in `modules_manager.go`, and return ModulesManager instance which contain all manager instances.

### Example
``` go
type ModulesManager struct {
    paymentManager *payment.PaymentManager
	orderManager *order.OrderManager
}

func NewModulesManager() *ModulesManager {
    serviceDB := database.NewRepoManager()

    paymentClient := payment_client.NewClient()
    paymentManager := payment.NewManager(paymentClient, serviceDB)

    orderClient := order_client.NewClient()
    orderManager := order.NewManager(orderClient, serviceDB, paymentManager)
	return &ModulesManager{
        paymentManager: paymentManager,
		orderManager: orderManager,
	}
}
```
