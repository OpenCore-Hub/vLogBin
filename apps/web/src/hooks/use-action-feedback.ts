"use client";

import { useActionState, useEffect, useRef } from "react";
import { useToast } from "@/components/ui/toast";

export interface ActionFeedbackState {
  ok?: boolean;
  error?: string;
}

export type ActionFeedbackAction<S extends ActionFeedbackState> = (
  prev: S,
  formData: FormData,
) => Promise<S>;

export interface UseActionFeedbackOptions<S extends ActionFeedbackState> {
  action: ActionFeedbackAction<S>;
  initialState: S;
  onSuccess?: (state: S) => void;
  onError?: (message: string) => void;
  successTitle?: string;
  successMessage?: (state: S) => string;
  errorTitle?: string;
}

/**
 * useActionState 的统一反馈封装：按 ActionState 约定集中处理成功/失败回调
 * 与可选 toast，避免每个表单重复维护 useEffect + 状态引用。
 */
export function useActionFeedback<S extends ActionFeedbackState>({
  action,
  initialState,
  onSuccess,
  onError,
  successTitle,
  successMessage,
  errorTitle,
}: UseActionFeedbackOptions<S>) {
  const { toast } = useToast();
  const [state, formAction, pending] = useActionState(
    action as (state: S, formData: FormData) => S | Promise<S>,
    initialState as Awaited<S>,
  );
  const handledRef = useRef(initialState);
  const callbacksRef = useRef({
    onSuccess,
    onError,
    successTitle,
    successMessage,
    errorTitle,
  });

  useEffect(() => {
    callbacksRef.current = {
      onSuccess,
      onError,
      successTitle,
      successMessage,
      errorTitle,
    };
  }, [onSuccess, onError, successTitle, successMessage, errorTitle]);

  useEffect(() => {
    if (state === handledRef.current) return;
    handledRef.current = state;
    const current = callbacksRef.current;
    if (state.ok) {
      if (current.successTitle) {
        toast({
          variant: "success",
          title: current.successTitle,
          description: current.successMessage?.(state),
        });
      }
      current.onSuccess?.(state);
    } else if (state.error) {
      if (current.errorTitle) {
        toast({
          variant: "danger",
          title: current.errorTitle,
          description: state.error,
        });
      }
      current.onError?.(state.error);
    }
  }, [state, toast]);

  return { state, formAction, pending };
}
