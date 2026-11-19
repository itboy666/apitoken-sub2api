/** Public contact data. Authentication and plugin management data never cross this bridge. */
export interface SupportWidgetConfig {
  enabled: boolean
  title: string
  description: string
  service_label: string
  service_contact: string
  service_url: string
  qq_group_number: string
  qq_group_url: string
  use_ticket: boolean
}

export interface PublicPluginDescriptor {
  id: number
  plugin_key: string
  name: string
  version: string
  entrypoint_url: string
  config: SupportWidgetConfig
}
