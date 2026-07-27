import { BaseAPI, route } from "../base";
import type { AssistantVoiceResponse } from "../types/data-contracts";

export type AssistantHistoryMessage = {
  role: string;
  content: string;
};

export class AssistantApi extends BaseAPI {
  /**
   * Send a recorded voice command. The server transcribes the audio, interprets
   * it with the configured AI provider, and returns the transcript, a natural
   * language reply and action proposals (never executed server-side).
   */
  voice(audio: Blob, history: AssistantHistoryMessage[] = []) {
    const formData = new FormData();
    formData.append("audio", audio, "recording.webm");
    if (history.length > 0) {
      formData.append("history", JSON.stringify(history));
    }

    return this.http.post<FormData, AssistantVoiceResponse>({
      url: route("/assistant/voice"),
      data: formData,
    });
  }
}
