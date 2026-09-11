//go:build darwin && !portless_helper

#import <Foundation/Foundation.h>
#import <ServiceManagement/ServiceManagement.h>
#include <stdlib.h>
#include <string.h>

#include "service_darwin.h"

static NSString *const STMPortlessServicePlist = @"pl.justcode.ssh-tunnel-manager.portless.plist";

static void stm_set_error(char **output, NSError *error) {
    if (output == NULL || error == nil) {
        return;
    }
    const char *message = error.localizedDescription.UTF8String;
    *output = strdup(message == NULL ? "unknown ServiceManagement error" : message);
}

int stm_portless_service_supported(void) {
    if (@available(macOS 13.0, *)) {
        return 1;
    }
    return 0;
}

int stm_portless_service_status(void) {
    if (@available(macOS 13.0, *)) {
        @autoreleasepool {
            SMAppService *service = [SMAppService daemonServiceWithPlistName:STMPortlessServicePlist];
            return (int)service.status;
        }
    }
    return -1;
}

int stm_portless_service_register(int replace_existing, char **error_message) {
    if (@available(macOS 13.0, *)) {
        @autoreleasepool {
            SMAppService *service = [SMAppService daemonServiceWithPlistName:STMPortlessServicePlist];
            if (replace_existing && service.status == SMAppServiceStatusEnabled) {
                NSError *unregisterError = nil;
                if (![service unregisterAndReturnError:&unregisterError]) {
                    service = [SMAppService daemonServiceWithPlistName:STMPortlessServicePlist];
                    // Another logged-in account may be refreshing the same
                    // machine-wide daemon concurrently. Treat the converged
                    // states as success and only surface a durable failure.
                    if (service.status == SMAppServiceStatusEnabled) {
                        return (int)service.status;
                    }
                    if (service.status != SMAppServiceStatusNotRegistered &&
                        service.status != SMAppServiceStatusNotFound) {
                        stm_set_error(error_message, unregisterError);
                        return -2;
                    }
                }
                service = [SMAppService daemonServiceWithPlistName:STMPortlessServicePlist];
            } else if (service.status == SMAppServiceStatusEnabled) {
                return (int)service.status;
            }

            NSError *registerError = nil;
            if (![service registerAndReturnError:&registerError]) {
                service = [SMAppService daemonServiceWithPlistName:STMPortlessServicePlist];
                // A parallel app instance may have completed registration.
                if (service.status == SMAppServiceStatusEnabled) {
                    return (int)service.status;
                }
                // A denied or still-pending approval is more actionable than
                // the generic registration error returned by the framework.
                if (service.status == SMAppServiceStatusRequiresApproval) {
                    return (int)service.status;
                }
                stm_set_error(error_message, registerError);
                return -2;
            }
            return (int)service.status;
        }
    }
    return -1;
}

int stm_portless_service_unregister(char **error_message) {
    if (@available(macOS 13.0, *)) {
        @autoreleasepool {
            SMAppService *service = [SMAppService daemonServiceWithPlistName:STMPortlessServicePlist];
            if (service.status == SMAppServiceStatusNotRegistered ||
                service.status == SMAppServiceStatusNotFound) {
                return 0;
            }
            NSError *error = nil;
            if (![service unregisterAndReturnError:&error]) {
                stm_set_error(error_message, error);
                return -2;
            }
            return 0;
        }
    }
    return -1;
}

void stm_portless_service_open_settings(void) {
    if (@available(macOS 13.0, *)) {
        [SMAppService openSystemSettingsLoginItems];
    }
}
