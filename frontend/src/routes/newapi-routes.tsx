import { NewAPIManagementPage } from "@/features/newapi-management/components/newapi-management-page";

export function NewAPIPlatformRoute() {
  return <NewAPIManagementPage view="platform" />;
}

export function NewAPIGroupsRoute() {
  return <NewAPIManagementPage view="groups" />;
}

export function NewAPIChannelsRoute() {
  return <NewAPIManagementPage view="channels" />;
}

export function NewAPIPricesRoute() {
  return <NewAPIManagementPage view="prices" />;
}

export function NewAPIDifferencesRoute() {
  return <NewAPIManagementPage view="differences" />;
}
