import type { KumaResource, KumaResourceList } from "@/api";
import { resourceDefaults } from "../../lib/resource-schemas";
import { config, monitor } from "./fixtures";

export const statusPage: KumaResource = {
  id: 10,
  name: "服务状态",
  type: "service",
  active: true,
  revision: "s1",
  association_revision: "a1",
  status_page: {
    ...resourceDefaults("status-pages").status_page,
    title: "服务状态",
    slug: "service",
    groups: [
      {
        id: 11,
        name: "API",
        monitorList: [
          { id: 19, sendUrl: false, url: "https://example.com/one" },
          { id: 20, sendUrl: true },
        ],
      },
      { id: 12, name: "备用", monitorList: [] },
    ],
  },
};
export const statusOptions: KumaResourceList = {
  config,
  items: [statusPage],
  monitors: [monitor, { ...monitor, id: 20, name: "备用接口" }],
  status_pages: [{ id: 10, title: "服务状态" }],
};
