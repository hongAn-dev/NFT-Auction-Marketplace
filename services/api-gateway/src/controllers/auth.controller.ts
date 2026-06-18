import { Controller, All, Req, Res, HttpStatus } from '@nestjs/common';
import { Request, Response } from 'express';

@Controller('api/auth')
export class AuthController {
  private readonly authServiceUrl = process.env.AUTH_SERVICE_URL || 'http://localhost:4001';

  @All('*')
  async proxyAuth(@Req() req: Request, @Res() res: Response) {
    const path = req.params[0] || '';
    const url = `${this.authServiceUrl}/auth/${path}`;
    
    try {
      const headers = { ...req.headers } as Record<string, string>;
      
      // Remove host and hop-by-hop/unsupported proxy headers
      delete headers.host;
      delete headers.connection;
      delete headers['content-length'];
      delete headers['Content-Length'];
      delete headers['transfer-encoding'];
      delete headers['Transfer-Encoding'];
      delete headers.expect;
      delete headers.Expect;
      
      const body = req.body && Object.keys(req.body).length > 0 ? JSON.stringify(req.body) : undefined;
      if (body) {
        headers['content-type'] = 'application/json';
      }

      console.log(`[Proxy] Forwarding request to: ${url} (${req.method})`);
      const response = await fetch(url, {
        method: req.method,
        headers,
        body,
      });

      const contentType = response.headers.get('content-type');
      let responseBody;
      
      if (contentType && contentType.includes('application/json')) {
        responseBody = await response.json();
      } else {
        responseBody = { success: response.ok, message: await response.text() };
      }
      
      return res.status(response.status).json(responseBody);
    } catch (error) {
      console.error(`[Proxy Error] Fail to fetch from ${url}:`, error);
      return res.status(HttpStatus.BAD_GATEWAY).json({
        success: false,
        error: {
          code: 'AUTH_SERVICE_UNREACHABLE',
          message: 'Dịch vụ xác thực hiện không khả dụng',
          details: error.message,
        },
      });
    }
  }
}
