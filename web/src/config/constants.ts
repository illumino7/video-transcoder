// Global environment configuration variables for the frontend application.
export const API_BASE = 'http://localhost:3030/api/v1';
export const MINIO_URL = 'http://localhost:9000';

// Enforce a strict file size ceiling on the client side to match the backend validation limit.
export const MAX_FILE_SIZE = 100 * 1024 * 1024; // 100 MB
