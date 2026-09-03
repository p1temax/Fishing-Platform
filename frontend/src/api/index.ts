import { apiClient } from "./client";

export const api = {
  login(data: { username: string; password: string }) {
    return apiClient.post("/api/auth/login/", data);
  },
  logout() {
    return apiClient.post("/api/auth/logout/");
  },
  getCurrentUser() {
    return apiClient.get("/api/auth/me/");
  },
  getUsers() {
    return apiClient.get("/api/users/");
  },
  createUser(data: { username: string; password: string; role?: string }) {
    return apiClient.post("/api/users/", data);
  },
  updateUser(id: number | string, data: { role?: string }) {
    return apiClient.put(`/api/users/${id}/`, data);
  },
  resetUserPassword(id: number | string) {
    return apiClient.post(`/api/users/${id}/reset-password/`);
  },
  deleteUser(id: number | string) {
    return apiClient.delete(`/api/users/${id}/`);
  },
  getDashboardStats() {
    return apiClient.get("/api/dashboard/");
  },
  getProjects() {
    return apiClient.get("/api/projects/");
  },
  createProject(data: FormData) {
    return apiClient.post("/api/projects/", data);
  },
  updateProject(id: number | string, data: FormData) {
    return apiClient.put(`/api/projects/${id}/`, data);
  },
  deleteProject(id: number | string) {
    return apiClient.delete(`/api/projects/${id}/`);
  },
  getProject(id: number | string) {
    return apiClient.get(`/api/projects/${id}/`);
  },
  startProject(id: number | string) {
    return apiClient.post(`/api/projects/${id}/start_container/`);
  },
  stopProject(id: number | string) {
    return apiClient.post(`/api/projects/${id}/stop_container/`);
  },
  startLocalProject(id: number | string) {
    return apiClient.post(`/api/projects/${id}/start_local/`);
  },
  stopLocalProject(id: number | string) {
    return apiClient.post(`/api/projects/${id}/stop_local/`);
  },
  buildProject(id: number | string, body: { force?: boolean } = {}) {
    return apiClient.post(`/api/projects/${id}/build/`, body);
  },
  getBuildStatus(id: number | string) {
    return apiClient.get(`/api/projects/${id}/build_status/`);
  },
  getProjectLogs(id: number | string) {
    return apiClient.get(`/api/projects/${id}/container_log/`);
  },
  getCentralLogs(id: number | string, limit = 500) {
    return apiClient.get(`/api/projects/${id}/logs/`, { params: { limit } });
  },
  getDeployments(id: number | string) {
    return apiClient.get(`/api/projects/${id}/deployments/`);
  },
  controlDeployment(id: number | string, deploymentId: number | string, action: string) {
    return apiClient.post(`/api/projects/${id}/deployments/${deploymentId}/${action}/`);
  },
  getCredentials(id: number | string) {
    return apiClient.get(`/api/projects/${id}/credentials/`, {
      params: { _ts: Date.now() },
    });
  },
  getAgents() {
    return apiClient.get("/api/agents/");
  },
  getAgent(id: number | string) {
    return apiClient.get(`/api/agents/${id}/`);
  },
  getRobots() {
    return apiClient.get("/api/robots/");
  },
  createRobot(data: unknown) {
    return apiClient.post("/api/robots/", data);
  },
  updateRobot(id: number | string, data: unknown) {
    return apiClient.put(`/api/robots/${id}/`, data);
  },
  deleteRobot(id: number | string) {
    return apiClient.delete(`/api/robots/${id}/`);
  },
  onlineRobot(id: number | string) {
    return apiClient.post(`/api/robots/${id}/online/`);
  },
  offlineRobot(id: number | string) {
    return apiClient.post(`/api/robots/${id}/offline/`);
  },
  testRobot(id: number | string, data: unknown) {
    return apiClient.post(`/api/robots/${id}/test/`, data);
  },
  getIPBlacklist() {
    return apiClient.get("/api/ip-blacklist/");
  },
  createIPBlacklistEntry(data: unknown) {
    return apiClient.post("/api/ip-blacklist/", data);
  },
  updateIPBlacklistEntry(id: number | string, data: unknown) {
    return apiClient.put(`/api/ip-blacklist/${id}/`, data);
  },
  deleteIPBlacklistEntry(id: number | string) {
    return apiClient.delete(`/api/ip-blacklist/${id}/`);
  },

  // SMTP services
  getSmtpServices() {
    return apiClient.get("/api/smtp-services/");
  },
  getSmtpService(id: number | string) {
    return apiClient.get(`/api/smtp-services/${id}/`);
  },
  createSmtpService(data: unknown) {
    return apiClient.post("/api/smtp-services/", data);
  },
  updateSmtpService(id: number | string, data: unknown) {
    return apiClient.put(`/api/smtp-services/${id}/`, data);
  },
  deleteSmtpService(id: number | string) {
    return apiClient.delete(`/api/smtp-services/${id}/`);
  },
  testSmtpService(id: number | string, data: unknown) {
    return apiClient.post(`/api/smtp-services/${id}/test/`, data);
  },
  sendSmtpServiceMail(id: number | string, data: unknown) {
    return apiClient.post(`/api/smtp-services/${id}/send/`, data);
  },

  // AI settings (admin)
  getAiSettings() {
    return apiClient.get("/api/ai-settings/");
  },
  updateAiSettings(data: unknown) {
    return apiClient.put("/api/ai-settings/", data);
  },
  getMailTrackingSettings() {
    return apiClient.get("/api/mail-tracking-settings/");
  },
  updateMailTrackingSettings(data: unknown) {
    return apiClient.put("/api/mail-tracking-settings/", data);
  },

  // Phishing pages (page builder)
  getPhishingPages() {
    return apiClient.get("/api/phishing-pages/");
  },
  getPhishingPage(id: number | string) {
    return apiClient.get(`/api/phishing-pages/${id}/`);
  },
  mirrorPhishingPage(data: {
    url: string;
    name?: string;
    description?: string;
    submit_url?: string;
    redirect_url?: string;
  }) {
    return apiClient.post("/api/phishing-pages/mirror/", data, { timeout: 180000 });
  },
  upsertPhishingPage(data: {
    name: string;
    html: string;
    url?: string;
    description?: string;
  }) {
    return apiClient.post("/api/phishing-pages/upsert/", data, { timeout: 120000 });
  },
  deletePhishingPage(id: number | string) {
    return apiClient.delete(`/api/phishing-pages/${id}/`);
  },

  // Audit logs (read-only)
  getAuditLogs(params?: {
    page?: number;
    page_size?: number;
    from?: string;
    to?: string;
    action?: string;
    actor?: string;
    keyword?: string;
  }) {
    return apiClient.get("/api/audit-logs/", { params });
  },
  getAuditLogActions() {
    return apiClient.get("/api/audit-logs/actions/");
  },

  // Mail campaigns (workbench)
  getMailCampaigns() {
    return apiClient.get("/api/mail-campaigns/");
  },
  getMailCampaign(id: number | string) {
    return apiClient.get(`/api/mail-campaigns/${id}/`);
  },
  createMailCampaign(data: unknown) {
    return apiClient.post("/api/mail-campaigns/", data);
  },
  getMailCampaignRecipients(id: number | string) {
    return apiClient.get(`/api/mail-campaigns/${id}/recipients/`);
  },
  getMailCampaignEvents(id: number | string) {
    return apiClient.get(`/api/mail-campaigns/${id}/events/`);
  },
};


export default api;
