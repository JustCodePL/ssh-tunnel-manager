#ifndef SSH_TUNNEL_MANAGER_PORTLESS_SERVICE_DARWIN_H
#define SSH_TUNNEL_MANAGER_PORTLESS_SERVICE_DARWIN_H

int stm_portless_service_supported(void);
int stm_portless_service_status(void);
int stm_portless_service_register(int replace_existing, char **error_message);
int stm_portless_service_unregister(char **error_message);
void stm_portless_service_open_settings(void);

#endif
