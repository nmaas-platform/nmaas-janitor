> ⚠️ **Archived project**: This repository is no longer maintained and retained **for reference purposes only**.

<div align="center">
  <a href="https://docs.nmaas.eu/">
    <img src="docs/nmaas-logo-blue.png" alt="Logo" width="206" height="48">
  </a>

<h3 align="center">nmaas Platform (Back-end)</h3>

<h4 align="center">Open-source multi-tenant platform for effortless, orchestrated deployment of software tools and applications on top of Kubernetes</h4>

  <p align="center">
    <br />
    <a href="https://docs.nmaas.eu/">Explore documentation</a>
    ·
    <a href="https://gitlab.software.geant.org/nmaas/nmaas-janitor/issues">Report Bug</a>
    ·
    <a href="https://gitlab.software.geant.org/nmaas/nmaas-janitor/issues">Request Feature</a>
  </p>
</div>

## 📦 Project archived

This project has been **archived** and is **no longer maintained**.

Starting with nmaas **release 1.8.0**, all features previously provided by this project were **re-implemented and integrated into the nmaas-platform** module. As a result, this repository is now **obsolete** and will not receive further updates, bug fixes, or support.

➡️ The actively maintained replacement of the provided features can be found within the **[nmaas-platform](https://gitlab.software.geant.org/nmaas/nmaas-platform)**.

This repository is retained **for reference purposes only**.

## nmaas Janitor Component

[nmaas Janitor](https://gitlab.software.geant.org/nmaas/nmaas-janitor) is a microservice that interacts with GitLab and Kubernetes API to perform various low level operations on behalf of the nmaas Platform

### Features

- Creating deployment ConfigMap(s) when configuration is pushed to GitLab repository
- Updating deployment ConfigMap(s) on demand
- Verifying deployment or statefulset status on demand
- Setting basic auth parameters on Ingress resources on demand
- Retrieving loadbalancer IP address assigned to given deployment or statefulset

### nmaas Janitor Development

Explore the nmaas Janitor [development and deployment](docs/DEVELOPMENT.md) documentation.

### Get in Touch

Interested users can use the following mailing lists to subscribe to news about nmaas, get in touch with the nmaas development team, or other nmaas users:

- [nmaas-announce@lists.geant.org](mailto:nmaas-announce@lists.geant.org) - public mailing list for announcements shared by the nmaas team with the community ([subscribe here](https://lists.geant.org/sympa/info/nmaas-announce))
- [nmaas@lists.geant.org](mailto:nmaas@lists.geant.org) - private mailing list for contacting the nmaas core team members
- [nmaas-users@lists.geant.org](mailto:nmaas-users@lists.geant.org) - public mailing lists for discussions related to nmaas usage and development ([subscribe here](https://lists.geant.org/sympa/info/nmaas-users))
