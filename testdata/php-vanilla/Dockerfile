# The vanilla PHP side of the load comparison: php 8.5 and apache with mod_php,
# serving the API testdata/testserver.php serves.
#
# mpm_prefork with a fixed worker count, which is the arrangement the phpscript
# side runs in: TESTSERVER_WORKERS requests answered at once behind a queue.
# conf/mpm.conf sets it, and VANILLA_WORKERS at build time is what changes it.
#
# No extension is installed. The API needs core and json alone, and php 8.5
# ships opcache compiled in, so conf/opcache.ini configures it and nothing
# builds it. A route that reads a database wants `docker-php-ext-install
# pdo_sqlite` here and nothing else.

# src/ is copied into this stage as well, because the autoloader is built with
# --classmap-authoritative and the classmap is read off those files: an
# autoloader generated without them would miss every class the handlers use.
FROM composer:2 AS vendor
WORKDIR /app
COPY composer.json composer.lock ./
COPY src/ ./src/
RUN composer install --no-dev --no-interaction --no-progress --classmap-authoritative --no-scripts

FROM php:8.5-apache

# mod_rewrite for the front controller: every path that is not a file on disk
# reaches public/index.php, which is where Slim's router takes over.
RUN a2enmod rewrite

ARG VANILLA_WORKERS=4
ARG VANILLA_PORT=8098

COPY conf/opcache.ini /usr/local/etc/php/conf.d/zz-opcache.ini
COPY conf/php.ini /usr/local/etc/php/conf.d/zz-testserver.ini
COPY conf/ports.conf /etc/apache2/ports.conf
COPY conf/vhost.conf /etc/apache2/sites-available/000-default.conf
COPY conf/mpm.conf /etc/apache2/mods-available/mpm_prefork.conf

# The build arguments reach the config apache parses at startup, which reads
# them from the environment. ENV and no literal, so one image serves whatever
# compose passes and the config file stays the same file.
ENV VANILLA_WORKERS=${VANILLA_WORKERS}
ENV VANILLA_PORT=${VANILLA_PORT}

WORKDIR /var/www/html
COPY public/ ./public/
COPY src/ ./src/
COPY --from=vendor /app/vendor ./vendor

# Where Slim writes its route cache, owned by the user apache runs as. The first
# request to find the file missing writes it, which is the warmup.
RUN mkdir -p var && chown www-data:www-data var
