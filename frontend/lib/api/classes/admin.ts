import { BaseAPI, route } from "../base";
import type { AdminResetLinkResponse, AdminUserCreateRequest, UserOut } from "../types/data-contracts";
import type { Result } from "../types/non-generated";

export class AdminApi extends BaseAPI {
  public listUsers() {
    return this.http.get<{ items: UserOut[] }>({ url: route("/admin/users") });
  }

  public createUser(body: AdminUserCreateRequest) {
    return this.http.post<AdminUserCreateRequest, Result<UserOut>>({ url: route("/admin/users"), body });
  }

  public deleteUser(id: string) {
    return this.http.delete<void>({ url: route(`/admin/users/${id}`) });
  }

  public resetLink(id: string) {
    return this.http.post<Record<string, never>, Result<AdminResetLinkResponse>>({
      url: route(`/admin/users/${id}/reset-link`),
      body: {},
    });
  }

  public setPassword(id: string, password: string) {
    return this.http.put<{ password: string }, void>({
      url: route(`/admin/users/${id}/password`),
      body: { password },
    });
  }

  public setDisabled(id: string, disabled: boolean) {
    return this.http.put<{ disabled: boolean }, void>({
      url: route(`/admin/users/${id}/disabled`),
      body: { disabled },
    });
  }
}
